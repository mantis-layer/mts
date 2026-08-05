import { defineConfig } from "vitepress";

const isGitHubPages = process.env.GITHUB_ACTIONS === "true";

export default defineConfig({
  lang: "zh-CN",
  title: "MTS",
  description: "模块化 Agent Runtime 与 SDK",
  base: isGitHubPages ? "/mts/" : "/",
  cleanUrls: true,
  lastUpdated: true,
  themeConfig: {
    logo: "/logo.svg",
    siteTitle: "MTS 文档",
    nav: [
      { text: "开始使用", link: "/guide/getting-started" },
      { text: "开发者", link: "/development/local-development" },
      { text: "模块", link: "/modules/overview" },
      { text: "GitHub", link: "https://github.com/mantis-layer/mts" }
    ],
    sidebar: {
      "/guide/": [
        {
          text: "入门",
          items: [
            { text: "概览", link: "/guide/getting-started" },
            { text: "核心概念", link: "/guide/concepts" },
            { text: "架构与边界", link: "/guide/architecture" }
          ]
        }
      ],
      "/development/": [
        {
          text: "开发者指南",
          items: [
            { text: "本地开发", link: "/development/local-development" },
            { text: "测试与验证", link: "/development/testing" },
            { text: "配置参考", link: "/development/configuration" }
          ]
        }
      ],
      "/modules/": [
        {
          text: "功能模块",
          items: [
            { text: "模块总览", link: "/modules/overview" },
            { text: "agent-model", link: "/modules/agent-model" },
            { text: "agent-core", link: "/modules/agent-core" },
            { text: "agent-runtime", link: "/modules/agent-runtime" },
            { text: "agent-plugin", link: "/modules/agent-plugin" },
            { text: "agent-compose", link: "/modules/agent-compose" },
            { text: "OpenAI 适配器", link: "/modules/model-openai" },
            { text: "内置工具、CLI 与示例", link: "/modules/tools-cli-examples" }
          ]
        }
      ]
    },
    search: { provider: "local" },
    socialLinks: [{ icon: "github", link: "https://github.com/mantis-layer/mts" }],
    footer: {
      message: "Released under the MIT License.",
      copyright: "Copyright © 2026 mantis-layer"
    },
    outline: { level: [2, 3], label: "本页目录" },
    editLink: {
      pattern: "https://github.com/mantis-layer/mts/edit/main/docs/:path",
      text: "在 GitHub 上编辑此页"
    }
  }
});
