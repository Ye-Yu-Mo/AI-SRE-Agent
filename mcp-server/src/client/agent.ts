type Config = {
  endpoint: string;
  secret: string;
};

let config: Config = {
  endpoint: process.env.AGENT_ENDPOINT || "http://localhost:9090",
  secret: process.env.AGENT_SECRET || "",
};

export function setConfig(c: Partial<Config>): void {
  config = { ...config, ...c };
}

export function getConfig(): Readonly<Config> {
  return config;
}

// AgentError 携带 HTTP status，让上层区分 409（需审批）和真错误。
// message 保持 `Agent API error: <status> <body>` 格式，向后兼容现有断言。
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
  private get base(): string {
    return config.endpoint;
  }

  private get headers(): Record<string, string> {
    return {
      Authorization: `Bearer ${config.secret}`,
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
