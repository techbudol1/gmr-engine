import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import {
  createPublicClient,
  createWalletClient,
  defineChain,
  encodeDeployData,
  getAddress,
  http,
  type Abi,
  type Address,
  type Hex,
} from "viem";
import { privateKeyToAccount } from "viem/accounts";

export const horizenTestnet = defineChain({
  id: 2651420,
  name: "Horizen Testnet",
  nativeCurrency: {
    decimals: 18,
    name: "Ether",
    symbol: "ETH",
  },
  rpcUrls: {
    default: {
      http: ["https://horizen-testnet.rpc.caldera.xyz/http"],
      webSocket: ["wss://horizen-testnet.rpc.caldera.xyz/ws"],
    },
  },
  blockExplorers: {
    default: {
      name: "Horizen Testnet Explorer",
      url: "https://horizen-testnet.explorer.caldera.xyz",
    },
  },
});

export type AADeployment = {
  accountAbstractionContractsPackage: string;
  bundlerUrl: string;
  chainId: number;
  chainName: string;
  deployedAt: string;
  deployer: Address;
  entryPoint: {
    address: Address;
    transactionHash?: Hex;
    version: "0.8";
  };
  explorerUrl: string;
  factory: {
    address: Address;
    transactionHash: Hex;
    type: "SimpleAccountFactory";
  };
  rpcUrl: string;
};

export type ContractArtifact = {
  abi: Abi;
  bytecode: Hex;
  contractName: string;
};

export const deploymentPath = resolve("deployments/horizen-aa-testnet.json");
const artifactsRoot = resolve("node_modules/@account-abstraction/contracts/artifacts");

export function loadArtifact(name: "EntryPoint" | "SimpleAccountFactory" | "SimpleAccount"): ContractArtifact {
  const artifactPath = resolve(artifactsRoot, `${name}.json`);
  const artifact = JSON.parse(readFileSync(artifactPath, "utf8")) as ContractArtifact;
  if (!artifact.abi || !artifact.bytecode || artifact.bytecode === "0x") {
    throw new Error(`invalid ${name} artifact at ${artifactPath}`);
  }
  return artifact;
}

export function loadDeployment(path = deploymentPath): AADeployment {
  if (!existsSync(path)) {
    throw new Error(`AA deployment file not found: ${path}`);
  }
  return JSON.parse(readFileSync(path, "utf8")) as AADeployment;
}

export function saveDeployment(deployment: AADeployment, path = deploymentPath) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, `${JSON.stringify(deployment, null, 2)}\n`);
}

export function rpcUrl() {
  return process.env.HORIZEN_AA_RPC_URL || process.env.GMR_ENGINE_RPC_URL || horizenTestnet.rpcUrls.default.http[0];
}

export function bundlerUrl() {
  return process.env.HORIZEN_AA_BUNDLER_URL || "";
}

export function ownerIndex() {
  const raw = process.env.HORIZEN_AA_ACCOUNT_INDEX || "0";
  if (!/^\d+$/.test(raw)) {
    throw new Error("HORIZEN_AA_ACCOUNT_INDEX must be a non-negative integer");
  }
  return BigInt(raw);
}

export function privateKeyEnv(name: string) {
  const raw = process.env[name]?.trim();
  if (!raw) {
    throw new Error(`${name} is required`);
  }
  const prefixed = raw.startsWith("0x") ? raw : `0x${raw}`;
  if (!/^0x[0-9a-fA-F]{64}$/.test(prefixed)) {
    throw new Error(`${name} must be a 32-byte hex private key`);
  }
  return prefixed as Hex;
}

export async function clients(privateKey?: Hex) {
  const transport = http(rpcUrl());
  const publicClient = createPublicClient({
    chain: horizenTestnet,
    transport,
  });
  const walletClient = privateKey
    ? createWalletClient({
        account: privateKeyToAccount(privateKey),
        chain: horizenTestnet,
        transport,
      })
    : undefined;
  return { publicClient, walletClient };
}

export async function assertHorizenChain() {
  const { publicClient } = await clients();
  const chainId = await publicClient.getChainId();
  if (chainId !== horizenTestnet.id) {
    throw new Error(`RPC returned chain ${chainId}; expected Horizen Testnet ${horizenTestnet.id}`);
  }
  return chainId;
}

export function normalizeAddress(address: string, label: string) {
  try {
    return getAddress(address) as Address;
  } catch {
    throw new Error(`${label} must be a valid EVM address`);
  }
}

export function deploymentEnv(deployment: AADeployment) {
  return [
    `HORIZEN_AA_ENABLED=true`,
    `HORIZEN_AA_CHAIN_ID=${deployment.chainId}`,
    `HORIZEN_AA_RPC_URL=${deployment.rpcUrl}`,
    `HORIZEN_AA_ENTRYPOINT_ADDRESS=${deployment.entryPoint.address}`,
    `HORIZEN_AA_ENTRYPOINT_VERSION=${deployment.entryPoint.version}`,
    `HORIZEN_AA_FACTORY_ADDRESS=${deployment.factory.address}`,
    `HORIZEN_AA_BUNDLER_URL=${deployment.bundlerUrl || "<set-after-bundler-is-running>"}`,
  ].join("\n");
}

export function entryPointDeployData() {
  const entryPoint = loadArtifact("EntryPoint");
  return encodeDeployData({
    abi: entryPoint.abi,
    bytecode: entryPoint.bytecode,
  });
}

export function factoryDeployData(entryPointAddress: Address) {
  const factory = loadArtifact("SimpleAccountFactory");
  return encodeDeployData({
    abi: factory.abi,
    args: [entryPointAddress],
    bytecode: factory.bytecode,
  });
}
