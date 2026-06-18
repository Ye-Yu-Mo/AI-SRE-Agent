import { readFileSync, writeFileSync, existsSync } from "fs";
import { join } from "path";

type Config = {
  endpoint: string;
  secret: string;
};

interface ServerEntry {
  id: string;
  endpoint: string;
  secret: string;
}

interface ServersFile {
  servers: ServerEntry[];
}

let config: Config = {
  endpoint: process.env.AGENT_ENDPOINT || "http://localhost:9090",
  secret: process.env.AGENT_SECRET || "",
};

// 服务器注册表：优先从 servers.json 加载，AGENT_ENDPOINTS 作为 fallback
const serversPath = join(process.cwd(), "servers.json");
let serverRegistry: ServerEntry[] = [];

function loadServers(): ServerEntry[] {
  const list: ServerEntry[] = [];

  // 1. servers.json 文件
  try {
    if (existsSync(serversPath)) {
      const data = JSON.parse(readFileSync(serversPath, "utf-8"));
      if (Array.isArray(data.servers)) {
        for (const s of data.servers) {
          if (s.id && s.endpoint && s.secret) {
            list.push({ id: s.id, endpoint: s.endpoint, secret: s.secret });
          }
        }
      }
    }
  } catch { /* skip */ }

  // 2. AGENT_ENDPOINTS 环境变量
  const envEndpoints = process.env.AGENT_ENDPOINTS || "";
  if (envEndpoints) {
    for (const entry of envEndpoints.split(";")) {
      const trimmed = entry.trim();
      if (!trimmed) continue;
      const eqIdx = trimmed.indexOf("=");
      if (eqIdx < 0) continue;
      const id = trimmed.slice(0, eqIdx).trim();
      const rest = trimmed.slice(eqIdx + 1);
      const commaIdx = rest.lastIndexOf(",");
      if (commaIdx < 0) continue;
      const endpoint = rest.slice(0, commaIdx).trim();
      const secret = rest.slice(commaIdx + 1).trim();
      if (id && endpoint && secret && !list.find(s => s.id === id)) {
        list.push({ id, endpoint, secret });
      }
    }
  }

  return list;
}

serverRegistry = loadServers();

function saveServers(list: ServerEntry[]): void {
  writeFileSync(serversPath, JSON.stringify({ servers: list }, null, 2) + "\n");
}

// 注册表查找
function findServer(id: string): ServerEntry | undefined {
  return serverRegistry.find(s => s.id === id);
}

// ListEndpoints 返回所有已注册服务器。
export function listEndpoints(): Array<{ id: string; endpoint: string }> {
  return serverRegistry.map(s => ({ id: s.id, endpoint: s.endpoint }));
}

// AddServer 添加一台服务器并持久化。
export function addServer(id: string, endpoint: string, secret: string): void {
  const existing = serverRegistry.findIndex(s => s.id === id);
  const entry: ServerEntry = { id, endpoint, secret };
  if (existing >= 0) {
    serverRegistry[existing] = entry;
  } else {
    serverRegistry.push(entry);
  }
  saveServers(serverRegistry);
}

// RemoveServer 删除一台服务器并持久化。
export function removeServer(id: string): boolean {
  const idx = serverRegistry.findIndex(s => s.id === id);
  if (idx < 0) return false;
  serverRegistry.splice(idx, 1);
  saveServers(serverRegistry);
  return true;
}

export function setConfig(c: Partial<Config>): void {
  config = { ...config, ...c };
}

export function getConfig(): Readonly<Config> {
  return config;
}

export class AgentError extends Error {
  status: number;
  body: string;
  constructor(status: number, body: string) {
    super(`Agent API error: ${status} ${body}`);
    this.name = "AgentError";
    this.status = status;
    this.body = body;
  }
}

export class AgentClient {
  private serverId: string | undefined;

  constructor(serverId?: string) {
    this.serverId = serverId;
  }

  private resolve(): { endpoint: string; secret: string } {
    // 1. server_id 精确匹配 registry
    if (this.serverId) {
      const found = findServer(this.serverId);
      if (found) return { endpoint: found.endpoint, secret: found.secret };
    }
    // 2. 没有 registry 也没有 server_id → 单服务器 fallback
    return { endpoint: config.endpoint, secret: config.secret };
  }

  private get base(): string {
    return this.resolve().endpoint;
  }

  private get headers(): Record<string, string> {
    return {
      Authorization: `Bearer ${this.resolve().secret}`,
      "Content-Type": "application/json",
    };
  }

  async get(path: string): Promise<any> {
    const url = `${this.base}${path}`;
    let res: Response;
    try {
      res = await fetch(url, {
        method: "GET",
        headers: this.headers,
        signal: AbortSignal.timeout(10_000),
      });
    } catch (err) {
      throw new Error(`Agent API error: ${(err as Error).message}`);
    }
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new AgentError(res.status, body);
    }
    return res.json();
  }

  async post(path: string, body: unknown): Promise<any> {
    const url = `${this.base}${path}`;
    let res: Response;
    try {
      res = await fetch(url, {
        method: "POST",
        headers: this.headers,
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(10_000),
      });
    } catch (err) {
      throw new Error(`Agent API error: ${(err as Error).message}`);
    }
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new AgentError(res.status, text);
    }
    return res.json();
  }
}
