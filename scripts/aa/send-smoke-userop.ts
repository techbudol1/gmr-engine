import { createSmartAccountClient } from "permissionless";
import { toSimpleSmartAccount } from "permissionless/accounts";
import { getAddress, http } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { bundlerUrl, clients, horizenTestnet, loadDeployment, ownerIndex, privateKeyEnv } from "./horizen-aa";

const ownerPrivateKey = privateKeyEnv("HORIZEN_AA_OWNER_PRIVATE_KEY");
const deployment = loadDeployment();
const bundler = bundlerUrl();

if (!bundler) {
  throw new Error("HORIZEN_AA_BUNDLER_URL is required to submit a UserOperation");
}

const owner = privateKeyToAccount(ownerPrivateKey);
const { publicClient } = await clients();

const account = await toSimpleSmartAccount({
  client: publicClient,
  entryPoint: {
    address: deployment.entryPoint.address,
    version: deployment.entryPoint.version,
  },
  factoryAddress: deployment.factory.address,
  index: ownerIndex(),
  owner,
});

const smartAccountAddress = await account.getAddress();
console.log(`Owner: ${getAddress(owner.address)}`);
console.log(`Smart account: ${getAddress(smartAccountAddress)}`);
console.log(`Bundler: ${bundler}`);

const smartAccountClient = createSmartAccountClient({
  account,
  bundlerTransport: http(bundler),
  chain: horizenTestnet,
  client: publicClient,
});

const userOperationHash = await smartAccountClient.sendTransaction({
  to: smartAccountAddress,
  value: 0n,
});

console.log(`Submitted smoke UserOperation: ${userOperationHash}`);
console.log("If this was the first operation, the SimpleAccount should now be deployed.");
