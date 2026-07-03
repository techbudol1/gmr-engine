import { createPublicClient, createWalletClient, defineChain, http, parseAbi, parseSignature } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { createVaultAccount, hasVaultSigner } from "./vault-account";

type Request = {
  amount: string;
  chainId: number;
  contractAddress: `0x${string}`;
  deadline: string;
  owner: `0x${string}`;
  privateKey?: `0x${string}`;
  recipient: `0x${string}`;
  rpcUrl: string;
  ownerVaultAddress?: `0x${string}`;
  ownerVaultApiKey?: string;
  ownerVaultProjectId?: string;
  ownerVaultUrl?: string;
  ownerVaultWalletRef?: string;
  r?: `0x${string}`;
  s?: `0x${string}`;
  spender: `0x${string}`;
  validateOnly?: boolean;
  v?: number;
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
const abi = parseAbi([
  "function name() view returns (string)",
  "function nonces(address owner) view returns (uint256)",
  "function permit(address owner, address spender, uint256 value, uint256 deadline, uint8 v, bytes32 r, bytes32 s) external",
  "function transferFrom(address from, address to, uint256 value) external returns (bool)",
]);

async function wait(kind: string, hash: `0x${string}`) {
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`${kind} reverted: ${hash}`);
  }
  return { kind, status: receipt.status, transactionHash: hash };
}

const value = BigInt(request.amount);
const deadline = BigInt(request.deadline);
let r = request.r;
let s = request.s;
let v = request.v;
let managedSignature: `0x${string}` | undefined;
let managedTypedData: any;
if (!r || !s || v === undefined) {
  const ownerAccount = createVaultAccount({
    chainId: request.chainId,
    rpcUrl: request.rpcUrl,
    vaultAddress: request.ownerVaultAddress,
    vaultApiKey: request.ownerVaultApiKey,
    vaultProjectId: request.ownerVaultProjectId,
    vaultUrl: request.ownerVaultUrl,
    vaultWalletRef: request.ownerVaultWalletRef,
  });
  if (ownerAccount.address.toLowerCase() !== request.owner.toLowerCase()) {
    throw new Error("managed owner does not match vault signer");
  }
  const [tokenName, nonce] = await Promise.all([
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "name" }),
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "nonces", args: [request.owner] }),
  ]);
  managedTypedData = {
    domain: {
      chainId: request.chainId,
      name: tokenName,
      verifyingContract: request.contractAddress,
      version: "1",
    },
    message: {
      deadline,
      nonce,
      owner: request.owner,
      spender: request.spender,
      value,
    },
    primaryType: "Permit",
    types: {
      EIP712Domain: [
        { name: "name", type: "string" },
        { name: "version", type: "string" },
        { name: "chainId", type: "uint256" },
        { name: "verifyingContract", type: "address" },
      ],
      Permit: [
        { name: "owner", type: "address" },
        { name: "spender", type: "address" },
        { name: "value", type: "uint256" },
        { name: "nonce", type: "uint256" },
        { name: "deadline", type: "uint256" },
      ],
    },
  };
  managedSignature = await (ownerAccount as any).signTypedData(managedTypedData);
  const parsed = parseSignature(managedSignature);
  r = parsed.r;
  s = parsed.s;
  v = Number(parsed.v ?? BigInt((parsed.yParity ?? 0) + 27));
}
if (request.validateOnly) {
  if (!managedSignature || !managedTypedData) throw new Error("managed signature validation requires a vault signer");
  const signatureValid = await publicClient.verifyTypedData({
    address: request.owner,
    ...managedTypedData,
    signature: managedSignature,
  });
  if (!signatureValid) throw new Error("managed permit signature is invalid");
  console.log(JSON.stringify({
    permitTransactionHash: "",
    signatureValid,
    transactions: [],
    transferTransactionHash: "",
  }));
  process.exit(0);
}
const permitHash = await walletClient.writeContract({
  abi,
  account,
  address: request.contractAddress,
  args: [request.owner, request.spender, value, deadline, v, r, s],
  functionName: "permit",
});
const permit = await wait("permit", permitHash);

const transferHash = await walletClient.writeContract({
  abi,
  account,
  address: request.contractAddress,
  args: [request.owner, request.recipient, value],
  functionName: "transferFrom",
});
const transfer = await wait("transferFrom", transferHash);

console.log(JSON.stringify({
  permitTransactionHash: permitHash,
  transferTransactionHash: transferHash,
  transactions: [permit, transfer],
}));
