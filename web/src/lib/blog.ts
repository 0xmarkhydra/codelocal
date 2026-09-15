export type BlogBlock =
  | { type: "paragraph"; text: string }
  | { type: "heading"; text: string }
  | { type: "list"; items: string[] }
  | { type: "code"; code: string; language?: string }
  | { type: "callout"; title: string; text: string }
  | { type: "image"; assetId: string; width: number; height: number; alt?: string; caption?: string; variant?: "medium" | "large" };

export type BlogAuthor = {
  slug: string;
  name: string;
  role: string;
};

export type BlogSeries = {
  slug: string;
  title: string;
  description: string;
  status: "active" | "complete";
  category: string;
  coverAssetId?: string;
  author?: BlogAuthor;
};

export type BlogPost = {
  slug: string;
  title: string;
  excerpt: string;
  publishedAt: string;
  updatedAt?: string;
  readingMinutes: number;
  category: string;
  tags: string[];
  author: BlogAuthor;
  featured?: boolean;
  showOnLanding?: boolean;
  official?: boolean;
  coverAssetId?: string;
  series?: { slug: string; part: number };
  blocks: BlogBlock[];
};

export const blogAuthors = {
  codelocal: {
    slug: "codelocal",
    name: "CodeLocal.Cloud Team",
    role: "Engineering & Product",
  },
} satisfies Record<string, BlogAuthor>;

export const blogSeries: BlogSeries[] = [
  {
    slug: "local-agent-foundations",
    title: "Local Agent Foundations",
    description:
      "A practical series about connecting AI coding clients to real projects while preserving workspace boundaries, verification and durable context.",
    status: "active",
    category: "Engineering",
    author: blogAuthors.codelocal,
  },
];

const posts: BlogPost[] = [
  {
    slug: "why-local-execution-matters-for-ai-coding-agents",
    title: "Why local execution still matters for AI coding agents",
    excerpt:
      "Cloud intelligence is useful, but source code, terminals and device credentials often belong behind a local execution boundary.",
    publishedAt: "2026-08-30",
    readingMinutes: 5,
    category: "Engineering",
    tags: ["Local AI", "Agents", "Security"],
    author: blogAuthors.codelocal,
    official: true,
    featured: true,
    series: { slug: "local-agent-foundations", part: 1 },
    blocks: [
      {
        type: "paragraph",
        text: "Modern coding agents can reason across large projects, call tools and carry work through multiple steps. The harder question is not whether an agent can act, but where those actions should actually run.",
      },
      {
        type: "heading",
        text: "Keep the execution boundary close to the project",
      },
      {
        type: "paragraph",
        text: "A local runtime lets the AI client request work while the machine that owns the workspace remains responsible for file access, terminal execution and device-level credentials. That separation reduces the amount of raw project state that must move elsewhere just to complete a task.",
      },
      {
        type: "list",
        items: [
          "Scope access to an explicitly granted workspace instead of the whole machine.",
          "Apply approval and security policy before sensitive actions execute.",
          "Return bounded results and evidence rather than treating unrestricted machine access as the default.",
        ],
      },
      {
        type: "callout",
        title: "The useful mental model",
        text: "The AI client proposes and coordinates. The authorized runtime decides what context and actions are allowed on the machine.",
      },
    ],
  },
  {
    slug: "connect-ai-clients-without-giving-up-workspace-control",
    title: "Connect AI clients without giving up workspace control",
    excerpt:
      "MCP can be the transport layer, but workspace scope, identity and approvals still need an explicit control plane.",
    publishedAt: "2026-08-30",
    readingMinutes: 6,
    category: "Guides",
    tags: ["MCP", "Agents", "Workspaces"],
    author: blogAuthors.codelocal,
    official: true,
    series: { slug: "local-agent-foundations", part: 2 },
    blocks: [
      {
        type: "paragraph",
        text: "Connecting an AI client to developer tools should not imply access to every repository, shell session or secret available on a computer. A useful integration begins with a logical project identity and a narrowly authorized workspace.",
      },
      {
        type: "heading",
        text: "Treat connection and permission as different things",
      },
      {
        type: "list",
        items: [
          "Pair the machine so requests can be routed to the intended runtime.",
          "Grant the specific workspace that the AI is allowed to understand and operate on.",
          "Let policy decide whether a requested action can run automatically or requires approval.",
          "Keep the transport stable so multiple compatible AI clients can reuse the same governed project boundary.",
        ],
      },
      {
        type: "code",
        language: "shell",
        code: "npm install -g codelocal@latest\n# Then pair the runtime and grant the intended workspace.",
      },
      {
        type: "callout",
        title: "Connection is not authorization",
        text: "An MCP endpoint can carry requests, but the runtime still needs its own rules for identity, project scope and sensitive operations.",
      },
    ],
  },
  {
    slug: "project-brain-durable-context-for-coding-agents",
    title: "Project Brain: durable context for coding agents",
    excerpt:
      "Useful project knowledge should survive across sessions without forcing every new AI conversation to rediscover the same decisions.",
    publishedAt: "2026-08-30",
    readingMinutes: 5,
    category: "Product",
    tags: ["Project Brain", "Context", "Agents"],
    author: blogAuthors.codelocal,
    official: true,
    series: { slug: "local-agent-foundations", part: 3 },
    blocks: [
      {
        type: "paragraph",
        text: "A long-running project contains more than source files. It also contains decisions, conventions, learned workflows, useful relationships and evidence about what has already worked. Losing that context between AI sessions creates repeated discovery work.",
      },
      {
        type: "heading",
        text: "Durable does not mean unbounded",
      },
      {
        type: "paragraph",
        text: "A project memory layer is most useful when it stores the smallest durable representation that can help future work. Raw secrets, unrestricted terminal history and machine-specific credentials do not need to become long-term AI context.",
      },
      {
        type: "list",
        items: [
          "Keep decisions and project rules that affect future implementation.",
          "Preserve verified experience with provenance instead of promoting guesses to fact.",
          "Retrieve only the context that is relevant to the current task.",
        ],
      },
      {
        type: "callout",
        title: "A better next session",
        text: "The goal is not maximum memory. The goal is less rediscovery, stronger continuity and a clear boundary between durable project knowledge and local-only state.",
      },
    ],
  },
  {
    slug: "a-practical-security-model-for-local-coding-agents",
    title: "A practical security model for local coding agents",
    excerpt:
      "A coding agent becomes much more useful when it can act, which makes scopes, approvals and verification part of the product—not optional plumbing.",
    publishedAt: "2026-08-30",
    readingMinutes: 7,
    category: "Security",
    tags: ["Security", "Approvals", "Local AI"],
    author: blogAuthors.codelocal,
    official: true,
    featured: true,
    blocks: [
      {
        type: "paragraph",
        text: "Read-only assistants are easy to reason about. Agentic systems are different because a useful request can turn into file writes, commands, browser actions or network calls. The security model has to govern the transition from understanding to execution.",
      },
      {
        type: "heading",
        text: "Build policy around capabilities",
      },
      {
        type: "list",
        items: [
          "Scope the agent to the intended workspace before exposing project tools.",
          "Separate ordinary project operations from sensitive actions that need explicit approval.",
          "Avoid exposing secrets to the model when a trusted local process can use them without printing them.",
          "Verify consequential changes with diagnostics, tests and diffs whenever practical.",
        ],
      },
      {
        type: "heading",
        text: "Make the safe path the convenient path",
      },
      {
        type: "paragraph",
        text: "Security controls that constantly interrupt routine work encourage users to bypass them. Good defaults should allow low-risk, scoped work to flow while preserving a clear approval boundary for actions that can materially affect the machine, data or external systems.",
      },
    ],
  },
];

export const blogPosts = [...posts].sort((a, b) => b.publishedAt.localeCompare(a.publishedAt));

export function taxonomySlug(value: string) {
  return value
    .trim()
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

export function formatBlogDate(value: string, locale = "en") {
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  }).format(new Date(`${value}T00:00:00Z`));
}

export function getPostBySlug(slug: string) {
  return blogPosts.find((post) => post.slug === slug);
}

export function getSeriesBySlug(slug: string) {
  return blogSeries.find((series) => series.slug === slug);
}

export function getSeriesPosts(slug: string) {
  return blogPosts
    .filter((post) => post.series?.slug === slug)
    .sort((a, b) => (a.series?.part ?? 0) - (b.series?.part ?? 0));
}

export function getCategories() {
  return [...new Set(blogPosts.map((post) => post.category))].sort();
}

export function getTags() {
  return [...new Set(blogPosts.flatMap((post) => post.tags))].sort();
}

export function getPostsByCategorySlug(slug: string) {
  return blogPosts.filter((post) => taxonomySlug(post.category) === slug);
}

export function getPostsByTagSlug(slug: string) {
  return blogPosts.filter((post) => post.tags.some((tag) => taxonomySlug(tag) === slug));
}

export function searchBlogPosts(query: string) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return blogPosts;

  return blogPosts.filter((post) =>
    [post.title, post.excerpt, post.category, ...post.tags]
      .join(" ")
      .toLowerCase()
      .includes(normalized),
  );
}

export function getRelatedPosts(post: BlogPost, limit = 3) {
  return blogPosts
    .filter((candidate) => candidate.slug !== post.slug)
    .map((candidate) => {
      const sharedTags = candidate.tags.filter((tag) => post.tags.includes(tag)).length;
      const sameCategory = candidate.category === post.category ? 2 : 0;
      const sameSeries = candidate.series?.slug && candidate.series.slug === post.series?.slug ? 3 : 0;
      return { candidate, score: sharedTags + sameCategory + sameSeries };
    })
    .sort((a, b) => b.score - a.score || b.candidate.publishedAt.localeCompare(a.candidate.publishedAt))
    .slice(0, limit)
    .map(({ candidate }) => candidate);
}
