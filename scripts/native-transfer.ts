import { createPublicClient, createWalletClient, defineChain, http } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { createVaultAccount, hasVaultSigner } from "./vault-account";

type Request = {
  amount: string;
  chainId: number;
  privateKey?: `0x${string}`;
  recipient: `0x${string}`;
  rpcUrl: string;
  vaultAddress?: `0x${string}`;
  vaultApiKey?: string;
  vaultProjectId?: string;
  vaultUrl?: string;
  vaultWalletRef?: string;
};

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body) as Request;
const chain = defineChain({
  id: request.chainId,
  name: `Chain ${request.chainId}`,
  nativeCurrency: { decimals: 18, name: "Ether", symbol: "ETH" },
  rpcUrls: {
    default: { http: [request.rpcUrl] },
  },
});
const transport = http(request.rpcUrl);
const publicClient = createPublicClient({ chain, transport });
if (!request.privateKey && !hasVaultSigner(request)) throw new Error("private key or vault signer is required");
const account = hasVaultSigner(request) ? createVaultAccount(request) : privateKeyToAccount(request.privateKey!);
const walletClient = createWalletClient({ account, chain, transport });
const value = BigInt(request.amount);
if (value <= 0n) throw new Error("amount must be greater than zero");

const hash = await walletClient.sendTransaction({
  account,
  chain,
  to: request.recipient,
  value,
});
const receipt = await publicClient.waitForTransactionReceipt({ hash });
if (receipt.status !== "success") {
  throw new Error(`native transfer reverted: ${hash}`);
}

console.log(JSON.stringify({
  transactionHash: hash,
  transactions: [{ kind: "native_transfer", status: receipt.status, transactionHash: hash }],
}));
