import { deploymentEnv, loadDeployment } from "./horizen-aa";

const deployment = loadDeployment();
console.log(deploymentEnv(deployment));
