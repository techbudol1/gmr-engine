import { getAddress, type Address } from "viem";
import { clients, deploymentPath, loadArtifact, loadDeployment, normalizeAddress, ownerIndex, rpcUrl } from "./horizen-aa";

const owner = normalizeAddress(process.argv[2] || process.env.HORIZEN_AA_OWNER_ADDRESS || "", "owner address");
const index = ownerIndex();
const deployment = loadDeployment();
const { publicClient } = await clients();
const factory = loadArtifact("SimpleAccountFactory");

const address = await publicClient.readContract({
  abi: factory.abi,
  address: deployment.factory.address,
  args: [owner, index],
  functionName: "getAddress",
}) as Address;

const code = await publicClient.getCode({ address });

console.log(JSON.stringify({
  accountIndex: index.toString(),
  deploymentFile: deploymentPath,
  entryPoint: deployment.entryPoint.address,
  factory: deployment.factory.address,
  owner: getAddress(owner),
  rpcUrl: rpcUrl(),
  smartAccount: getAddress(address),
  status: code && code !== "0x" ? "deployed" : "counterfactual",
}, null, 2));
