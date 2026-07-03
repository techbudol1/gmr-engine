export type GmrEngineOptions = {
  baseUrl?: string;
  secretKey: string;
};

export type GmrTransaction = {
  id: string;
  chainId: number;
  walletAddress: string;
  contractAddress: string;
  kind: string;
  method: string;
  args: string[];
  status: string;
  transactionHash?: string;
};

type RequestOptions = {
  body?: unknown;
  method?: string;
};

export class GmrEngineClient {
  private readonly baseUrl: string;
  private readonly secretKey: string;

  constructor(options: GmrEngineOptions) {
    this.baseUrl = options.baseUrl ?? "http://localhost:8090";
    this.secretKey = options.secretKey;
  }

  async createUserWallet(input: {
    address: string;
    authProvider?: string;
    email?: string;
    metadata?: Record<string, unknown>;
    userId: string;
  }) {
    const response = await this.request<{ wallet: unknown }>("/v1/user-wallets", {
      body: input,
      method: "POST",
    });
    return response.wallet;
  }

  async getERC20Balance(input: {
    address: string;
    chainId: number;
    contractAddress: string;
    decimals?: number;
  }) {
    const search = new URLSearchParams({
      address: input.address,
      chainId: String(input.chainId),
      contractAddress: input.contractAddress,
    });
    if (input.decimals) {
      search.set("decimals", String(input.decimals));
    }
    return this.request<{ balance: string; balanceRaw: string; symbol: string }>(`/v1/erc20/balance?${search}`);
  }

  async transferERC20(input: {
    amount: string;
    chainId: number;
    contractAddress: string;
    decimals?: number;
    recipient: string;
  }) {
    return this.request<{ transactions: GmrTransaction[] }>("/v1/erc20/transfer", {
      body: input,
      method: "POST",
    });
  }

  async transferERC20WithPermit(input: {
    amount: string;
    chainId: number;
    contractAddress: string;
    deadline: string;
    decimals?: number;
    owner: string;
    recipient: string;
    r: string;
    s: string;
    v: number;
  }) {
    return this.request<{
      gasFree: boolean;
      permitTransactionHash: string;
      transactionIds: string[];
      transferTransactionHash: string;
    }>("/v1/erc20/transfer-with-permit", {
      body: input,
      method: "POST",
    });
  }

  async deployPredictionEscrow(input: {
    chainId: number;
    description?: string;
    feeBps?: number;
    name?: string;
    ownerAddress: string;
    tokenAddress: string;
    treasuryAddress: string;
  }) {
    const response = await this.request<{ deployment: unknown }>("/v1/contracts/escrow/deployments", {
      body: input,
      method: "POST",
    });
    return response.deployment;
  }

  async readContract<T = unknown>(input: {
    abi: unknown[];
    args?: string[];
    chainId: number;
    contractAddress: string;
    functionName: string;
  }): Promise<T> {
    const response = await this.request<{ result: T }>("/v1/contracts/read", {
      body: { ...input, args: input.args ?? [] },
      method: "POST",
    });
    return response.result;
  }

  async writeContract(input: {
    abi: unknown[];
    args?: string[];
    chainId: number;
    contractAddress: string;
    functionName: string;
    value?: string;
    walletAddress?: string;
  }): Promise<GmrTransaction> {
    const response = await this.request<{ transaction: GmrTransaction }>("/v1/contracts/write", {
      body: { ...input, args: input.args ?? [] },
      method: "POST",
    });
    return response.transaction;
  }

  async getTransaction(id: string): Promise<GmrTransaction> {
    const response = await this.request<{ transaction: GmrTransaction }>(`/v1/transactions/${encodeURIComponent(id)}`);
    return response.transaction;
  }

  private async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      body: options.body ? JSON.stringify(options.body) : undefined,
      headers: {
        "Content-Type": "application/json",
        "X-GMR-Engine-Key": this.secretKey,
      },
      method: options.method ?? "GET",
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new Error(error.error ?? `GMR Engine request failed with ${response.status}`);
    }
    return response.json() as Promise<T>;
  }
}
