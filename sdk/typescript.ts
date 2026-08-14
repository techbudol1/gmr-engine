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

export type SolanaNetwork = "mainnet" | "devnet";

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

  async getSolanaBalance(input: { network: SolanaNetwork; walletAddress: string }) {
    const search = new URLSearchParams(input);
    return this.request<{ balance: { decimals: 9; network: SolanaNetwork; raw: string; slot: number; symbol: "SOL"; value: string; walletAddress: string } }>(`/v1/solana/native/balance?${search}`);
  }

  async getSPLBalance(input: { mintAddress: string; network: SolanaNetwork; walletAddress: string }) {
    const search = new URLSearchParams(input);
    return this.request<{ balance: { decimals: number; mintAddress: string; raw: string; value: string } }>(`/v1/solana/spl/balance?${search}`);
  }

  async getSPLBalances(input: { network: SolanaNetwork; walletAddress: string }) {
    const search = new URLSearchParams(input);
    return this.request<{ balances: Array<{ decimals: number; mintAddress: string; raw: string; value: string }> }>(`/v1/solana/spl/balances?${search}`);
  }

  async getSolanaTransactions(input: { before?: string; limit?: number; network: SolanaNetwork; walletAddress: string }) {
    const search = new URLSearchParams({ network: input.network, walletAddress: input.walletAddress });
    if (input.before) search.set("before", input.before);
    if (input.limit) search.set("limit", String(input.limit));
    return this.request<{ transactions: Array<{ blockTime?: number; confirmationStatus: string; err: unknown; memo: unknown; signature: string; slot: number }> }>(`/v1/solana/wallets/transactions?${search}`);
  }

  async getSolanaAccount<T = unknown>(input: { address: string; network: SolanaNetwork }) {
    const search = new URLSearchParams({ network: input.network });
    return this.request<{ account: T }>(`/v1/solana/accounts/${encodeURIComponent(input.address)}?${search}`);
  }

  async getSolanaLatestBlockhash(input: { network: SolanaNetwork }) {
    const search = new URLSearchParams(input);
    return this.request<{ blockhash: { context: { slot: number }; value: { blockhash: string; lastValidBlockHeight: number } } }>(`/v1/solana/blockhash/latest?${search}`);
  }

  async simulateSolanaTransaction(input: { commitment?: "processed" | "confirmed" | "finalized"; network: SolanaNetwork; transaction: string }) {
    return this.request<{ simulation: unknown }>("/v1/solana/transactions/simulate", { body: input, method: "POST" });
  }

  async sendSolanaTransaction(input: { commitment?: "processed" | "confirmed" | "finalized"; network: SolanaNetwork; transaction: string }) {
    return this.request<{ network: SolanaNetwork; signature: string; status: "submitted" }>("/v1/solana/transactions/send", { body: input, method: "POST" });
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
