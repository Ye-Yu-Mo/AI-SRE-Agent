type Config = {
  endpoint: string;
  secret: string;
};

let config: Config = {
  endpoint: process.env.AGENT_ENDPOINT || "http://localhost:9090",
  secret: process.env.AGENT_SECRET || "",
};

// M4: 多服务器路由。AGENT_ENDPOINTS 格式：
//   srv_01=http://1.2.3.4:9090,secret1;srv_02=http://5.6.7.8:9090,secret2
// 若不配则回退到 AGENT_ENDPOINT + AGENT_SECRET（单服务器模式）。
const endpointsMap = parseEndpoints(process.env.AGENT_ENDPOINTS || "");

function parseEndpoints(raw: string): Map<string, { endpoint: string; secret: string }> {
  const m = new Map<string, { endpoint: string; secret: string }>();
  if (!raw) return m;
  for (const entry of raw.split(";")) {
    const trimmed = entry.trim();
    if (!trimmed) continue;
    const eqIdx = trimmed.indexOf("=");
    if (eqIdx < 0) continue;
    const sid = trimmed.slice(0, eqIdx).trim();
    const rest = trimmed.slice(eqIdx + 1);
    const commaIdx = rest.lastIndexOf(",");
    if (commaIdx < 0) continue;
    const endpoint = rest.slice(0, commaIdx).trim();
    const secret = rest.slice(commaIdx + 1).trim();
    if (sid && endpoint && secret) {
      m.set(sid, { endpoint, secret });
    }
  }
  return m;
}

export function setConfig(c: Partial<Config>): void {
  config = { ...config, ...c };
}

export function getConfig(): Readonly<Config> {
  return config;
}

// AgentError 携带 HTTP status，让上层区分 409（需审批）和真错误。
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

  // resolve 返回最终使用的 endpoint 和 secret。优先 AGENT_ENDPOINTS，回退单服务器。
  private resolve(): { endpoint: string; secret: string } {
    if (this.serverId && endpointsMap.has(this.serverId)) {
      return endpointsMap.get(this.serverId)!;
    }
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
