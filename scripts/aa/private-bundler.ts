import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createPublicClient, createWalletClient, decodeEventLog, encodeAbiParameters, formatGwei, getAddress, http, keccak256, parseAbiItem, toEventHash, toHex, type Address, type Hex } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { getPackedUserOperation } from "permissionless/utils";
import { horizenTestnet, privateKeyEnv } from "./horizen-aa";
import { createVaultAccount } from "../vault-account";

type JsonRpcRequest = {
  id?: number | string | null;
  jsonrpc?: string;
  method?: string;
  params?: unknown[];
};

type UserOperationInput = Record<string, unknown>;

type UserOperationRecord = {
  entryPoint: Address;
  sender: Address;
  nonce: Hex;
  transactionHash: Hex;
  userOperation: UserOperationInput;
  userOperationHash: Hex;
};

const entryPointAbi = JSON.parse(readFileSync(resolve("node_modules/@account-abstraction/contracts/artifacts/EntryPoint.json"), "utf8")).abi;
const userOperationEvent = parseAbiItem("event UserOperationEvent(bytes32 indexed userOpHash, address indexed sender, address indexed paymaster, uint256 nonce, bool success, uint256 actualGasCost, uint256 actualGasUsed)");
const userOperationEventTopic = toEventHash(userOperationEvent);

const rpcUrl = env("HORIZEN_AA_RPC_URL", env("GMR_ENGINE_RPC_URL", horizenTestnet.rpcUrls.default.http[0]));
const entryPointAddress = normalizeAddress(env("HORIZEN_AA_ENTRYPOINT_ADDRESS"), "HORIZEN_AA_ENTRYPOINT_ADDRESS");
const account = bundlerAccount();
const beneficiary = normalizeAddress(env("HORIZEN_BUNDLER_BENEFICIARY_ADDRESS", account.address), "HORIZEN_BUNDLER_BENEFICIARY_ADDRESS");
const port = Number(env("HORIZEN_BUNDLER_PORT", "8092"));
const allowedOrigins = env("HORIZEN_BUNDLER_ALLOWED_ORIGINS", "*").split(",").map(origin => origin.trim()).filter(Boolean);
const defaultCallGasLimit = BigInt(env("HORIZEN_BUNDLER_DEFAULT_CALL_GAS_LIMIT", "250000"));
const defaultVerificationGasLimit = BigInt(env("HORIZEN_BUNDLER_DEFAULT_VERIFICATION_GAS_LIMIT", "700000"));
const defaultPreVerificationGas = BigInt(env("HORIZEN_BUNDLER_DEFAULT_PRE_VERIFICATION_GAS", "80000"));
const paymasterAddress = optionalAddress(env("HORIZEN_AA_PAYMASTER_ADDRESS"), "HORIZEN_AA_PAYMASTER_ADDRESS");
const paymasterVerificationGasLimit = BigInt(env("HORIZEN_AA_PAYMASTER_VERIFICATION_GAS_LIMIT", "250000"));
const paymasterPostOpGasLimit = BigInt(env("HORIZEN_AA_PAYMASTER_POST_OP_GAS_LIMIT", "60000"));
const paymasterValidSeconds = BigInt(env("HORIZEN_AA_PAYMASTER_VALID_SECONDS", "300"));

const publicClient = createPublicClient({
  chain: horizenTestnet,
  transport: http(rpcUrl),
});
const walletClient = createWalletClient({
  account,
  chain: horizenTestnet,
  transport: http(rpcUrl),
});
const operations = new Map<string, UserOperationRecord>();

const chainId = await publicClient.getChainId();
if (chainId !== horizenTestnet.id) {
  throw new Error(`Bundler RPC returned chain ${chainId}; expected ${horizenTestnet.id}`);
}
const entryPointCode = await publicClient.getBytecode({ address: entryPointAddress });
if (!entryPointCode || entryPointCode === "0x") {
  throw new Error(`EntryPoint has no code: ${entryPointAddress}`);
}
const bundlerBalance = await publicClient.getBalance({ address: account.address });
console.log(JSON.stringify({
  beneficiary,
  bundler: getAddress(account.address),
  bundlerBalanceWei: bundlerBalance.toString(),
  chainId,
  entryPoint: entryPointAddress,
  gasPriceGwei: formatGwei(await publicClient.getGasPrice()),
  port,
  rpcUrl,
  service: "budolph-private-erc4337-bundler",
}));

Bun.serve({
  async fetch(request) {
    if (request.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders(request) });
    }
    if (request.method === "GET" && new URL(request.url).pathname === "/healthz") {
      return json({ beneficiary, bundler: getAddress(account.address), chainId, entryPoint: entryPointAddress, ok: true }, request);
    }
    if (request.method !== "POST") {
      return jsonRpcError(null, -32600, "Only POST JSON-RPC requests are supported", request);
    }
    let payload: JsonRpcRequest | JsonRpcRequest[];
    try {
      payload = await request.json();
    } catch {
      return jsonRpcError(null, -32700, "Invalid JSON", request);
    }
    if (Array.isArray(payload)) {
      const responses = await Promise.all(payload.map(item => handleRpc(item, request)));
      return json(responses, request);
    }
    return json(await handleRpc(payload, request), request);
  },
  port,
});

async function handleRpc(payload: JsonRpcRequest, request: Request) {
  const id = payload.id ?? null;
  try {
    switch (payload.method) {
      case "eth_chainId":
        return rpcResult(id, toHex(horizenTestnet.id));
      case "net_version":
        return rpcResult(id, String(horizenTestnet.id));
      case "web3_clientVersion":
        return rpcResult(id, "budolph-private-erc4337-bundler/0.1");
      case "eth_supportedEntryPoints":
        return rpcResult(id, [entryPointAddress]);
      case "eth_estimateUserOperationGas":
        return rpcResult(id, await estimateUserOperationGas(payload.params));
      case "pimlico_getUserOperationGasPrice":
        return rpcResult(id, await userOperationGasPrice());
      case "pm_sponsorUserOperation":
        return rpcResult(id, await sponsorUserOperation(payload.params));
      case "eth_sendUserOperation":
        return rpcResult(id, await sendUserOperation(payload.params));
      case "eth_getUserOperationReceipt":
        return rpcResult(id, await getUserOperationReceipt(payload.params));
      case "eth_getUserOperationByHash":
        return rpcResult(id, await getUserOperationByHash(payload.params));
      default:
        return rpcError(id, -32601, `Unsupported method: ${payload.method || ""}`);
    }
  } catch (error) {
    return rpcError(id, -32000, error instanceof Error ? error.message : "Bundler request failed");
  }
}

async function estimateUserOperationGas(params: unknown[] | undefined) {
  const [rawUserOp, rawEntryPoint] = params || [];
  assertEntryPoint(rawEntryPoint);
  const op = normalizeUserOperation(rawUserOp as UserOperationInput, true);
  let callGasLimit = op.callGasLimit;
  try {
    if (op.callData !== "0x") {
      callGasLimit = await publicClient.estimateContractGas({
        abi: entryPointAbi,
        address: entryPointAddress,
        args: [[getPackedUserOperation(withGasDefaults(op))], beneficiary],
        functionName: "handleOps",
      });
    }
  } catch {
    callGasLimit = defaultCallGasLimit;
  }
  return {
    callGasLimit: toHex(maxBigInt(callGasLimit, defaultCallGasLimit)),
    preVerificationGas: toHex(defaultPreVerificationGas),
    verificationGasLimit: toHex(defaultVerificationGasLimit),
  };
}

async function userOperationGasPrice() {
  const gasPrice = await publicClient.getGasPrice();
  return {
    fast: { maxFeePerGas: toHex(gasPrice * 2n), maxPriorityFeePerGas: toHex(gasPrice) },
    slow: { maxFeePerGas: toHex(gasPrice), maxPriorityFeePerGas: toHex(gasPrice / 2n || 1n) },
    standard: { maxFeePerGas: toHex((gasPrice * 3n) / 2n), maxPriorityFeePerGas: toHex(gasPrice / 2n || 1n) },
  };
}

async function sponsorUserOperation(params: unknown[] | undefined) {
  if (!paymasterAddress) {
    throw new Error("paymaster is not configured");
  }
  const [rawUserOp, rawEntryPoint] = params || [];
  assertEntryPoint(rawEntryPoint);
  const op = normalizeUserOperation(rawUserOp as UserOperationInput, true);
  const packed = getPackedUserOperation({
    ...op,
    paymaster: undefined,
    paymasterData: "0x",
    paymasterPostOpGasLimit: undefined,
    paymasterVerificationGasLimit: undefined,
  });
  const validUntil = BigInt(Math.floor(Date.now() / 1000)) + paymasterValidSeconds;
  const sponsorHash = keccak256(encodeAbiParameters(
    [
      { name: "paymaster", type: "address" },
      { name: "chainId", type: "uint256" },
      { name: "sender", type: "address" },
      { name: "nonce", type: "uint256" },
      { name: "initCodeHash", type: "bytes32" },
      { name: "callDataHash", type: "bytes32" },
      { name: "accountGasLimits", type: "bytes32" },
      { name: "preVerificationGas", type: "uint256" },
      { name: "gasFees", type: "bytes32" },
      { name: "validUntil", type: "uint256" },
    ],
    [
      paymasterAddress,
      BigInt(horizenTestnet.id),
      op.sender,
      op.nonce,
      keccak256(packed.initCode),
      keccak256(packed.callData),
      packed.accountGasLimits,
      packed.preVerificationGas,
      packed.gasFees,
      validUntil,
    ],
  ));
  const signature = await account.signMessage({ message: { raw: sponsorHash } });
  return {
    paymaster: paymasterAddress,
    paymasterData: `${uint256Hex(validUntil)}${signature.slice(2)}` as Hex,
    paymasterPostOpGasLimit: toHex(paymasterPostOpGasLimit),
    paymasterVerificationGasLimit: toHex(paymasterVerificationGasLimit),
  };
}

async function sendUserOperation(params: unknown[] | undefined) {
  const [rawUserOp, rawEntryPoint] = params || [];
  assertEntryPoint(rawEntryPoint);
  const userOperation = normalizeUserOperation(rawUserOp as UserOperationInput, false);
  const packed = getPackedUserOperation(userOperation);
  const userOperationHash = await publicClient.readContract({
    abi: entryPointAbi,
    address: entryPointAddress,
    args: [packed],
    functionName: "getUserOpHash",
  }) as Hex;
  const transactionHash = await walletClient.writeContract({
    abi: entryPointAbi,
    address: entryPointAddress,
    args: [[packed], beneficiary],
    functionName: "handleOps",
  });
  operations.set(userOperationHash.toLowerCase(), {
    entryPoint: entryPointAddress,
    nonce: toHex(userOperation.nonce),
    sender: userOperation.sender,
    transactionHash,
    userOperation: rawUserOp as UserOperationInput,
    userOperationHash,
  });
  return userOperationHash;
}

async function getUserOperationReceipt(params: unknown[] | undefined) {
  const hash = normalizeHash(String((params || [])[0] || ""), "userOperationHash");
  const record = operations.get(hash.toLowerCase());
  if (!record) {
    return null;
  }
  const receipt = await publicClient.getTransactionReceipt({ hash: record.transactionHash }).catch(() => null);
  if (!receipt) {
    return null;
  }
  const userOpLog = receipt.logs.find(log => log.topics[0]?.toLowerCase() === userOperationEventTopic.toLowerCase() && log.topics[1]?.toLowerCase() === hash.toLowerCase());
  let success = receipt.status === "success";
  let actualGasCost = "0x0";
  let actualGasUsed = toHex(receipt.gasUsed);
  let paymaster = "0x0000000000000000000000000000000000000000";
  if (userOpLog) {
    try {
      const decoded = decodeEventLog({
        abi: [userOperationEvent],
        data: userOpLog.data,
        topics: userOpLog.topics,
      });
      const args = decoded.args as unknown as { actualGasCost: bigint; actualGasUsed: bigint; paymaster: Address; success: boolean };
      success = args.success;
      actualGasCost = toHex(args.actualGasCost);
      actualGasUsed = toHex(args.actualGasUsed);
      paymaster = args.paymaster;
    } catch {
      // Keep receipt-level fallback values.
    }
  }
  return {
    actualGasCost,
    actualGasUsed,
    entryPoint: record.entryPoint,
    logs: receipt.logs,
    nonce: record.nonce,
    paymaster,
    receipt,
    sender: record.sender,
    success,
    userOpHash: record.userOperationHash,
  };
}

async function getUserOperationByHash(params: unknown[] | undefined) {
  const hash = normalizeHash(String((params || [])[0] || ""), "userOperationHash");
  const record = operations.get(hash.toLowerCase());
  if (!record) {
    return null;
  }
  return {
    blockHash: null,
    blockNumber: null,
    entryPoint: record.entryPoint,
    transactionHash: record.transactionHash,
    userOperation: record.userOperation,
  };
}

function normalizeUserOperation(raw: UserOperationInput, allowPartial: boolean) {
  if (!raw || typeof raw !== "object") {
    throw new Error("userOperation is required");
  }
  const op = raw as Record<string, unknown>;
  const normalized = {
    callData: hex(op.callData, allowPartial ? "0x" : undefined, "callData"),
    callGasLimit: uint(op.callGasLimit, allowPartial ? defaultCallGasLimit : undefined, "callGasLimit"),
    factory: optionalAddress(op.factory, "factory"),
    factoryData: hex(op.factoryData, "0x", "factoryData"),
    maxFeePerGas: uint(op.maxFeePerGas, allowPartial ? 1n : undefined, "maxFeePerGas"),
    maxPriorityFeePerGas: uint(op.maxPriorityFeePerGas, allowPartial ? 1n : undefined, "maxPriorityFeePerGas"),
    nonce: uint(op.nonce, allowPartial ? 0n : undefined, "nonce"),
    paymaster: optionalAddress(op.paymaster, "paymaster"),
    paymasterData: hex(op.paymasterData, "0x", "paymasterData"),
    paymasterPostOpGasLimit: optionalUint(op.paymasterPostOpGasLimit, "paymasterPostOpGasLimit"),
    paymasterVerificationGasLimit: optionalUint(op.paymasterVerificationGasLimit, "paymasterVerificationGasLimit"),
    preVerificationGas: uint(op.preVerificationGas, allowPartial ? defaultPreVerificationGas : undefined, "preVerificationGas"),
    sender: normalizeAddress(String(op.sender || ""), "sender"),
    signature: hex(op.signature, allowPartial ? "0x" : undefined, "signature"),
    verificationGasLimit: uint(op.verificationGasLimit, allowPartial ? defaultVerificationGasLimit : undefined, "verificationGasLimit"),
  };
  const initCode = String(op.initCode || "");
  if (!normalized.factory && /^0x[0-9a-fA-F]+$/.test(initCode) && initCode.length >= 42) {
    normalized.factory = getAddress(`0x${initCode.slice(2, 42)}`) as Address;
    normalized.factoryData = `0x${initCode.slice(42)}` as Hex;
  }
  return withGasDefaults(normalized);
}

function withGasDefaults(op: ReturnType<typeof normalizeUserOperation>) {
  return {
    ...op,
    callGasLimit: op.callGasLimit || defaultCallGasLimit,
    preVerificationGas: op.preVerificationGas || defaultPreVerificationGas,
    verificationGasLimit: op.verificationGasLimit || defaultVerificationGasLimit,
  };
}

function assertEntryPoint(value: unknown) {
  const address = normalizeAddress(String(value || ""), "entryPoint");
  if (address.toLowerCase() !== entryPointAddress.toLowerCase()) {
    throw new Error(`unsupported EntryPoint ${address}; expected ${entryPointAddress}`);
  }
}

function env(key: string, fallback = "") {
  return process.env[key]?.trim() || fallback;
}

function bundlerAccount() {
  const vaultWalletRef = env("HORIZEN_BUNDLER_VAULT_WALLET_REF");
  if (vaultWalletRef) {
    return createVaultAccount({
      chainId: horizenTestnet.id,
      rpcUrl,
      vaultAddress: normalizeAddress(env("HORIZEN_BUNDLER_ADDRESS"), "HORIZEN_BUNDLER_ADDRESS"),
      vaultApiKey: env("HORIZEN_BUNDLER_VAULT_API_KEY", env("GMR_ENGINE_VAULT_INTERNAL_API_KEY")),
      vaultUrl: env("HORIZEN_BUNDLER_VAULT_URL", env("GMR_ENGINE_VAULT_URL")),
      vaultWalletRef,
    });
  }
  return privateKeyToAccount(privateKeyEnv("HORIZEN_BUNDLER_PRIVATE_KEY"));
}

function normalizeAddress(value: string, label: string) {
  try {
    return getAddress(value) as Address;
  } catch {
    throw new Error(`${label} must be a valid EVM address`);
  }
}

function normalizeHash(value: string, label: string) {
  if (!/^0x[0-9a-fA-F]{64}$/.test(value)) {
    throw new Error(`${label} must be a 32-byte hex string`);
  }
  return value as Hex;
}

function optionalAddress(value: unknown, label: string) {
  if (value === undefined || value === null || value === "" || value === "0x") {
    return undefined;
  }
  return normalizeAddress(String(value), label);
}

function hex(value: unknown, fallback: Hex | undefined, label: string) {
  if (value === undefined || value === null || value === "") {
    if (fallback !== undefined) {
      return fallback;
    }
    throw new Error(`${label} is required`);
  }
  const raw = String(value);
  if (!/^0x[0-9a-fA-F]*$/.test(raw)) {
    throw new Error(`${label} must be hex`);
  }
  return raw as Hex;
}

function optionalUint(value: unknown, label: string) {
  if (value === undefined || value === null || value === "") {
    return undefined;
  }
  return uint(value, undefined, label);
}

function uint(value: unknown, fallback: bigint | undefined, label: string) {
  if (value === undefined || value === null || value === "") {
    if (fallback !== undefined) {
      return fallback;
    }
    throw new Error(`${label} is required`);
  }
  try {
    const parsed = typeof value === "bigint" ? value : BigInt(String(value));
    if (parsed < 0n) {
      throw new Error("negative");
    }
    return parsed;
  } catch {
    throw new Error(`${label} must be an unsigned integer`);
  }
}

function maxBigInt(a: bigint, b: bigint) {
  return a > b ? a : b;
}

function uint256Hex(value: bigint) {
  return toHex(value, { size: 32 }) as Hex;
}

function rpcResult(id: JsonRpcRequest["id"], result: unknown) {
  return { id, jsonrpc: "2.0", result };
}

function rpcError(id: JsonRpcRequest["id"], code: number, message: string) {
  return { error: { code, message }, id, jsonrpc: "2.0" };
}

function jsonRpcError(id: JsonRpcRequest["id"], code: number, message: string, request: Request) {
  return json(rpcError(id, code, message), request, 400);
}

function json(payload: unknown, request: Request, status = 200) {
  return new Response(JSON.stringify(payload), {
    headers: {
      "content-type": "application/json",
      ...corsHeaders(request),
    },
    status,
  });
}

function corsHeaders(request: Request) {
  const origin = request.headers.get("origin") || "";
  const allowOrigin = allowedOrigins.includes("*") || !origin
    ? "*"
    : allowedOrigins.includes(origin)
      ? origin
      : allowedOrigins[0] || "null";
  return {
    "access-control-allow-headers": "content-type, authorization",
    "access-control-allow-methods": "POST, OPTIONS, GET",
    "access-control-allow-origin": allowOrigin,
    "cache-control": "no-store",
  };
}
