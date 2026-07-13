import { toAccount } from "viem/accounts";

type VaultSignerRequest = {
  chainId: number;
  rpcUrl: string;
  vaultAddress?: `0x${string}`;
  vaultApiKey?: string;
  vaultProjectId?: string;
  vaultUrl?: string;
  vaultWalletRef?: string;
};

type VaultTransaction = {
  chainId?: number | bigint | string;
  data?: `0x${string}`;
  gas?: bigint;
  gasLimit?: bigint;
  gasPrice?: bigint;
  maxFeePerGas?: bigint;
  maxPriorityFeePerGas?: bigint;
  nonce?: number;
  to?: `0x${string}`;
  value?: bigint;
};

export function hasVaultSigner(request: VaultSignerRequest) {
  return Boolean(request.vaultWalletRef && request.vaultUrl && request.vaultApiKey && request.vaultAddress);
}

export function createVaultAccount(request: VaultSignerRequest) {
  const walletId = parseVaultWalletId(request.vaultWalletRef);
  const vaultUrl = request.vaultUrl?.replace(/\/+$/, "");
  const vaultApiKey = request.vaultApiKey;
  const address = request.vaultAddress;
  if (!walletId || !vaultUrl || !vaultApiKey || !address) {
    throw new Error("vault wallet reference, URL, key, and address are required");
  }

  return toAccount({
    address,
    async signMessage({ message }) {
      const raw = typeof message === "string"
        ? message
        : typeof message.raw === "string"
          ? message.raw
          : bytesToHex(message.raw);
      const isHex = typeof raw === "string" && raw.startsWith("0x");
      const response = await callVault(vaultUrl, vaultApiKey, "/v1/sign-message", {
        walletId,
        message: raw,
        format: isHex ? "hex" : "text",
      });
      return response.signature;
    },
    async signTransaction(transaction: VaultTransaction) {
      const response = await callVault(vaultUrl, vaultApiKey, "/v1/sign-transaction", {
        walletId,
        transaction: {
          chainId: normalizeQuantity(transaction.chainId ?? request.chainId),
          data: transaction.data ?? "0x",
          gasLimit: Number(transaction.gas ?? transaction.gasLimit ?? 0n),
          gasPrice: normalizeQuantity(transaction.gasPrice),
          maxFeePerGas: normalizeQuantity(transaction.maxFeePerGas),
          maxPriorityFeePerGas: normalizeQuantity(transaction.maxPriorityFeePerGas),
          nonce: transaction.nonce ?? 0,
          to: transaction.to ?? "",
          value: normalizeQuantity(transaction.value),
        },
      });
      return response.signedTransaction.rawTransaction;
    },
    async signTypedData(typedData) {
      const response = await callVault(vaultUrl, vaultApiKey, "/v1/sign-typed-data", {
        walletId,
        typedData,
      });
      return response.signature;
    },
  });
}

function parseVaultWalletId(ref?: string) {
  const value = String(ref ?? "").trim();
  const prefix = "gmr-vault:v1:";
  if (!value.startsWith(prefix)) return "";
  return value.slice(prefix.length).trim();
}

function normalizeQuantity(value: unknown) {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "bigint") return value.toString();
  return String(value);
}

function bytesToHex(value: Uint8Array) {
  return `0x${Array.from(value).map(byte => byte.toString(16).padStart(2, "0")).join("")}`;
}

async function callVault(vaultUrl: string, vaultApiKey: string, path: string, payload: unknown) {
  const response = await fetch(`${vaultUrl}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-GMR-Vault-Key": vaultApiKey,
    },
    body: JSON.stringify(payload, (_key, value) => typeof value === "bigint" ? value.toString() : value),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `vault request failed: ${response.status}`);
  }
  return data;
}
