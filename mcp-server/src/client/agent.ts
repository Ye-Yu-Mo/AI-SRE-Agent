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
      throw new Error(`Agent API error: ${res.status} ${body}`);
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
      throw new Error(`Agent API error: ${res.status} ${text}`);
    }
    return res.json();
  }
}
