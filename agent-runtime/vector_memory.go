package agentruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mantis-layer/mts/agent-contract"
	agentmodel "github.com/mantis-layer/mts/agent-model"

	_ "modernc.org/sqlite"     // 纯 Go SQLite driver（无 CGO）
	_ "modernc.org/sqlite/vec" // 纯 Go sqlite-vec 扩展（余弦相似度 KNN）
)

// VectorMemoryStore 是 FR-011 的默认 MemoryStore 实现：
// SQLite（modernc.org/sqlite，纯 Go、零 CGO）+ sqlite-vec 扩展做余弦相似度 Top-K。
//
// 它与 Storage（FR-007，TaskRun 持久化）完全解耦——有独立的 sqlite 连接与
// schema，不实现 Storage 接口，只实现 agentcontract.MemoryStore。
//
// 分层语义：
//   - Working 层不做 embedding（Memory.Embedding 为 nil）。Query 走规则检索：
//     按 created_at 倒序 + opts 过滤。
//   - ShortTerm/LongTerm/Preference/Skill 层在 Save 时由 EmbeddingProvider 生成
//     向量（若 provider 非 nil），Query 时对查询文本生成向量做余弦相似度 Top-K。
//
// 退化路径：若某条记忆未带 embedding（如 provider 为 nil 或 Save 时未提供），
// 检索仍按 created_at 倒序回退（记忆是增强，不破坏 Run 终态）。
type VectorMemoryStore struct {
	db       *sql.DB
	embed    agentmodel.EmbeddingProvider // 可为 nil：无向量化能力
	dim      int                          // embedding 维度；0 表示尚未确定（首条 embedding 时冻结）
	vecReady bool                         // vec0 虚拟表是否已按 dim 建好
}

// NewVectorMemoryStore 打开（必要时创建）SQLite 文件并初始化 schema。
// embed 可为 nil（此时所有层都退化为规则检索）；dim 为预期向量维度，
// 用于建 vec0 表与校验；传 0 则推迟到首条带 embedding 的记忆写入时确定。
func NewVectorMemoryStore(path string, embed agentmodel.EmbeddingProvider, dim int) (*VectorMemoryStore, error) {
	if path == "" {
		return nil, fmt.Errorf("agentruntime: vector store path 不能为空")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(memoryDir(path), 0o755); err != nil {
			return nil, fmt.Errorf("agentruntime: 创建目录: %w", err)
		}
	}
	dsn := path + "?_pragma=busy_timeout(10000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: 打开 vector store: %w", err)
	}
	// 单写者模型（与 SQLiteStorage 一致），避免并发写 "database is locked"。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("agentruntime: vector store ping: %w", err)
	}
	s := &VectorMemoryStore{db: db, embed: embed, dim: dim}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if dim > 0 {
		if err := s.ensureVecTable(dim); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return s, nil
}

func memoryDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func (s *VectorMemoryStore) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			persona_id TEXT NOT NULL,
			layer TEXT NOT NULL,
			content TEXT NOT NULL,
			metadata_json TEXT,
			tags_json TEXT,
			embedding_json TEXT,
			created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_persona_layer ON memories(persona_id, layer)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("agentruntime: vector store schema: %w", err)
		}
	}
	return nil
}

// ensureVecTable 幂等地按给定维度创建 vec0 虚拟表（余弦相似度）。
// vec0 要求维度在建表时固定；维度确定后，后续记忆的 embedding 维度必须一致。
func (s *VectorMemoryStore) ensureVecTable(dim int) error {
	if dim <= 0 {
		return fmt.Errorf("agentruntime: vec 维度必须 > 0，实际 %d", dim)
	}
	if s.vecReady && s.dim == dim {
		return nil
	}
	// 若已有不同维度的 vec 表，先 drop（仅在初始化重建场景；运行期维度漂移由 Save 校验拦截）。
	if s.vecReady && s.dim != dim {
		return fmt.Errorf("agentruntime: vec 维度冲突（已建 %d，请求 %d）", s.dim, dim)
	}
	ddl := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_vec USING vec0(memory_id TEXT, embedding float[%d], +distance_metric=cosine)`,
		dim,
	)
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("agentruntime: 建 vec0 表（dim=%d）: %w", dim, err)
	}
	s.dim = dim
	s.vecReady = true
	return nil
}

// Close 关闭底层 SQLite 连接。持久化数据保留在文件中（跨会话恢复）。
func (s *VectorMemoryStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Save 写入一条记忆（FR-011）。
//
//   - Working 层：跳过 embedding（nil embedding，规则检索）。
//   - 其他层：若 m.Embedding 已提供，直接用并校验维度；否则若 EmbeddingProvider
//     非 nil，由 provider 对 Content 生成向量；两者都不可得则该条记忆不带向量
//     （检索退化为 created_at 倒序）。
//
// 写入失败返回错误，不改变已存数据（事务保证）。EmbeddingProvider 调用失败
// 也返回错误（记忆是增强，不应静默吞掉 provider 故障）。
func (s *VectorMemoryStore) Save(ctx context.Context, m *agentcontract.Memory) error {
	if m == nil {
		return fmt.Errorf("agentruntime: Save 收到 nil Memory")
	}
	if err := m.Validate(); err != nil {
		return err
	}

	emb := m.Embedding
	needsEmbed := m.Layer != agentcontract.MemoryLayerWorking && len(emb) == 0

	// 向量生成：provider 对 Content 嵌入。
	if needsEmbed && s.embed != nil {
		vecs, err := s.embed.Embed(ctx, []string{m.Content})
		if err != nil {
			return fmt.Errorf("agentruntime: 生成 embedding 失败: %w", err)
		}
		if len(vecs) != 1 || len(vecs[0]) == 0 {
			return fmt.Errorf("agentruntime: embedding provider 返回空向量")
		}
		emb = vecs[0]
	}

	// 维度冻结与校验。
	if len(emb) > 0 {
		if s.dim > 0 && len(emb) != s.dim {
			return fmt.Errorf("agentruntime: 向量维度不一致（期望 %d，实际 %d）", s.dim, len(emb))
		}
		if s.dim == 0 {
			if err := s.ensureVecTable(len(emb)); err != nil {
				return err
			}
		}
	}

	metadataJSON, _ := json.Marshal(m.Metadata)
	tagsJSON, _ := json.Marshal(m.Tags)
	var embeddingJSON []byte
	if len(emb) > 0 {
		embeddingJSON, _ = json.Marshal(emb)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("agentruntime: vector store begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 成功 Commit 后 Rollback 是 no-op

	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO memories (id, persona_id, layer, content, metadata_json, tags_json, embedding_json, created_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		m.ID, m.PersonaID, string(m.Layer), m.Content,
		nullable(metadataJSON), nullable(tagsJSON), nullable(embeddingJSON),
		m.CreatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("agentruntime: vector store save memory: %w", err)
	}

	// 更新向量索引（存在 vec 表且该记忆有 embedding）。
	if s.vecReady {
		if len(emb) > 0 && m.Layer != agentcontract.MemoryLayerWorking {
			// 先删旧行（INSERT OR REPLACE 在 memories 表上，但 vec 表无对应语义）。
			if _, err := tx.ExecContext(ctx, `DELETE FROM memories_vec WHERE memory_id = ?`, m.ID); err != nil {
				return fmt.Errorf("agentruntime: vector store upsert (delete old vec): %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO memories_vec (memory_id, embedding) VALUES (?, ?)`,
				m.ID, vecJSON(emb),
			); err != nil {
				return fmt.Errorf("agentruntime: vector store insert vec: %w", err)
			}
		} else {
			// 该记忆不带向量（Working 层或无 embedding），从 vec 表清除可能的旧向量。
			if _, err := tx.ExecContext(ctx, `DELETE FROM memories_vec WHERE memory_id = ?`, m.ID); err != nil {
				return fmt.Errorf("agentruntime: vector store clear vec: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("agentruntime: vector store commit: %w", err)
	}
	return nil
}

// Query 检索记忆（FR-011）。
//
//   - Working 层：规则检索——按 created_at 倒序 + opts.Tags 过滤。
//   - 其他层：若 store 配置了 EmbeddingProvider，则对 opts.QueryText（或空字符串）
//     生成查询向量，做余弦相似度 Top-K；若无 provider/无向量数据，退化为 created_at 倒序。
//
// 无结果返回空切片（非 nil）。limit <= 0 时取默认上限 20。
func (s *VectorMemoryStore) Query(ctx context.Context, personaID string, layer agentcontract.MemoryLayer, opts agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
	if err := layer.Validate(); err != nil {
		return nil, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// 向量检索路径：有 provider 且 layer 非 Working。
	if layer != agentcontract.MemoryLayerWorking && s.embed != nil && s.vecReady {
		return s.queryVector(ctx, personaID, layer, opts, limit)
	}
	return s.queryRules(ctx, personaID, layer, opts, limit)
}

// queryRules 按 created_at 倒序 + Tags 过滤。
func (s *VectorMemoryStore) queryRules(ctx context.Context, personaID string, layer agentcontract.MemoryLayer, opts agentcontract.QueryOptions, limit int) ([]agentcontract.Memory, error) {
	q := `SELECT id, persona_id, layer, content, metadata_json, tags_json, embedding_json, created_at
	      FROM memories WHERE persona_id = ? AND layer = ?`
	args := []any{personaID, string(layer)}
	if len(opts.Tags) > 0 {
		// 简单子串匹配：tags_json 内含每个 tag（小规模数据足够，避免 JSON 查询复杂度）。
		for _, t := range opts.Tags {
			q += ` AND tags_json LIKE ?`
			args = append(args, "%\""+likeEscape(t)+"\"%")
		}
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: vector store query: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// queryVector 用查询向量做余弦相似度 Top-K，再回表读取完整 Memory。
func (s *VectorMemoryStore) queryVector(ctx context.Context, personaID string, layer agentcontract.MemoryLayer, opts agentcontract.QueryOptions, limit int) ([]agentcontract.Memory, error) {
	queryText := opts.QueryText
	if queryText == "" {
		// 无查询文本：退化为规则检索（按时间倒序）。
		return s.queryRules(ctx, personaID, layer, opts, limit)
	}
	vecs, err := s.embed.Embed(ctx, []string{queryText})
	if err != nil {
		return nil, fmt.Errorf("agentruntime: 生成查询向量失败: %w", err)
	}
	if len(vecs) != 1 || len(vecs[0]) == 0 || s.dim > 0 && len(vecs[0]) != s.dim {
		return nil, fmt.Errorf("agentruntime: 查询向量维度异常")
	}

	// KNN：先取 Top-K*候选（向量库内做），再在 Go 内按 persona/layer/tag 精过滤，
	// 不足时整体相似度排序结果可能 < limit（符合预期：相似记忆不够多就少返回）。
	// 取候选池放大以补偿过滤损耗。
	pool := limit * 4
	if pool < limit {
		pool = limit
	}
	knn, err := s.db.QueryContext(ctx,
		`SELECT memory_id, distance FROM memories_vec
		 WHERE embedding MATCH ? ORDER BY distance LIMIT ?`,
		vecJSON(vecs[0]), pool)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: vector store knn: %w", err)
	}
	type hit struct {
		id   string
		dist float64
	}
	var hits []hit
	for knn.Next() {
		var h hit
		if err := knn.Scan(&h.id, &h.dist); err != nil {
			knn.Close()
			return nil, fmt.Errorf("agentruntime: vector store knn scan: %w", err)
		}
		hits = append(hits, h)
	}
	if err := knn.Err(); err != nil {
		knn.Close()
		return nil, fmt.Errorf("agentruntime: vector store knn rows: %w", err)
	}
	knn.Close()
	if len(hits) == 0 {
		return []agentcontract.Memory{}, nil
	}

	// 回表读取完整记忆，按相似度（hits 顺序）优先级匹配。
	idSet := make(map[string]int, len(hits))
	for i, h := range hits {
		idSet[h.id] = i
	}
	ids := make([]string, len(hits))
	for _, h := range hits {
		ids[idSet[h.id]] = h.id
	}

	// 按 ids 顺序查询（用 IN），再在 Go 内过滤 persona/layer/tag 并排序。
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(ids)+2)
	args = append(args, personaID, string(layer))
	for _, id := range ids {
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT id, persona_id, layer, content, metadata_json, tags_json, embedding_json, created_at
		 FROM memories WHERE persona_id = ? AND layer = ? AND id IN (%s)`,
		placeholders,
	)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: vector store fetch: %w", err)
	}
	defer rows.Close()
	got, err := scanMemories(rows)
	if err != nil {
		return nil, err
	}

	// Go 内 tag 过滤。
	var filtered []agentcontract.Memory
	for _, m := range got {
		if !tagsMatch(m.Tags, opts.Tags) {
			continue
		}
		filtered = append(filtered, m)
	}
	// 按相似度（hits 顺序）排序——got 已按 IN 顺序未必保序，显式重排。
	rank := func(id string) int { return idSet[id] }
	// 稳定排序：按 rank 升序。
	sortByRank(filtered, rank)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// Delete 按 id 删除一条记忆（含其向量）。
func (s *VectorMemoryStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("agentruntime: Delete 收到空 id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("agentruntime: vector store delete begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if s.vecReady {
		if _, err := tx.ExecContext(ctx, `DELETE FROM memories_vec WHERE memory_id = ?`, id); err != nil {
			return fmt.Errorf("agentruntime: vector store delete vec: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id); err != nil {
		return fmt.Errorf("agentruntime: vector store delete memory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("agentruntime: vector store delete commit: %w", err)
	}
	return nil
}

// scanMemories 扫描行集为 []Memory。
func scanMemories(rows *sql.Rows) ([]agentcontract.Memory, error) {
	var out []agentcontract.Memory
	for rows.Next() {
		var m agentcontract.Memory
		var layer, createdAt string
		var metadataJSON, tagsJSON, embeddingJSON sql.NullString
		if err := rows.Scan(&m.ID, &m.PersonaID, &layer, &m.Content, &metadataJSON, &tagsJSON, &embeddingJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("agentruntime: vector store scan: %w", err)
		}
		m.Layer = agentcontract.MemoryLayer(layer)
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if metadataJSON.Valid && metadataJSON.String != "" {
			_ = json.Unmarshal([]byte(metadataJSON.String), &m.Metadata)
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			_ = json.Unmarshal([]byte(tagsJSON.String), &m.Tags)
		}
		if embeddingJSON.Valid && embeddingJSON.String != "" {
			_ = json.Unmarshal([]byte(embeddingJSON.String), &m.Embedding)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agentruntime: vector store rows: %w", err)
	}
	if out == nil {
		out = []agentcontract.Memory{}
	}
	return out, nil
}

// vecJSON 把 []float32 序列化为 sqlite-vec 接受的 JSON 数组字符串。
func vecJSON(v []float32) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// tagsMatch 报告 memory 的 tags 是否包含所有 query tags（空 query 视为匹配）。
func tagsMatch(memory, query []string) bool {
	if len(query) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(memory))
	for _, t := range memory {
		set[t] = struct{}{}
	}
	for _, q := range query {
		if _, ok := set[q]; !ok {
			return false
		}
	}
	return true
}

// likeEscape 转义 LIKE 的特殊字符，避免注入/误匹配。
func likeEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// sortByRank 按 rank(id) 升序稳定排序 memories。
func sortByRank(ms []agentcontract.Memory, rank func(string) int) {
	// 插入排序（小规模，稳定）。
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && rank(ms[j].ID) < rank(ms[j-1].ID); j-- {
			ms[j], ms[j-1] = ms[j-1], ms[j]
		}
	}
}

// 编译期接口满足性检查（A1）：VectorMemoryStore 实现 agentcontract.MemoryStore。
var _ agentcontract.MemoryStore = (*VectorMemoryStore)(nil)
