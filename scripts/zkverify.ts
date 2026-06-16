import {
  CurveType,
  Library,
  Plonky2HashFunction,
  Risc0Version,
  TeeVariant,
  UltrahonkVariant,
  UltrahonkVersion,
  zkVerifySession,
} from "zkverifyjs";

type Request = {
  action: "accountInfo" | "submitProof";
  domainId?: number;
  network?: string;
  proof?: unknown;
  proofSystem?: string;
  publicSignals?: unknown;
  rpcUrl?: string;
  seedPhrase?: string;
  version?: string;
  variant?: string;
  vk?: unknown;
  websocketUrl?: string;
};

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body) as Request;

const seedPhrase = request.seedPhrase || process.env.GMR_ENGINE_ZKVERIFY_SEED_PHRASE || "";
const websocket = request.websocketUrl || process.env.GMR_ENGINE_ZKVERIFY_WS_URL || "wss://testnet-rpc.zkverify.io";
const rpc = request.rpcUrl || process.env.GMR_ENGINE_ZKVERIFY_RPC_URL || "https://testnet-rpc.zkverify.io";
const network = request.network || process.env.GMR_ENGINE_ZKVERIFY_NETWORK || "Volta";

if (!seedPhrase) {
  throw new Error("ZKVerify seed phrase is required");
}

const session = await zkVerifySession
  .start()
  .Custom({ network, rpc, websocket })
  .withAccount(seedPhrase);

try {
  if (request.action === "accountInfo") {
    const accounts = await session.getAccountInfo();
    console.log(JSON.stringify({ accounts }));
    process.exit(0);
  }

  if (request.action !== "submitProof") {
    throw new Error(`unsupported action: ${request.action}`);
  }

  const proofSystem = (request.proofSystem || "groth16").toLowerCase();
  const proofData = {
    proof: request.proof,
    publicSignals: request.publicSignals,
    vk: request.vk,
  };
  const executeInput = {
    proofData,
    ...(request.domainId !== undefined ? { domainId: request.domainId } : {}),
  };

  const base = session.verify();
  let verifier: any;
  switch (proofSystem) {
    case "groth16":
      verifier = base.groth16({ curve: CurveType.bn254, library: Library.snarkjs });
      break;
    case "ultraplonk":
      verifier = base.ultraplonk({ numberOfPublicInputs: Array.isArray(request.publicSignals) ? request.publicSignals.length : 0 });
      break;
    case "ultrahonk":
      verifier = base.ultrahonk({
        variant: request.variant === "zk" ? UltrahonkVariant.ZK : UltrahonkVariant.Plain,
        version: request.version === "v3" ? UltrahonkVersion.V3_0 : UltrahonkVersion.V0_84,
      });
      break;
    case "risc0":
      verifier = base.risc0({ version: Risc0Version.V3_0 });
      break;
    case "sp1":
      verifier = base.sp1();
      break;
    case "fflonk":
      verifier = base.fflonk();
      break;
    case "ezkl":
      verifier = base.ezkl();
      break;
    case "plonky2":
      verifier = base.plonky2({ hashFunction: Plonky2HashFunction.Poseidon });
      break;
    case "tee":
      verifier = base.tee({ variant: TeeVariant.Intel });
      break;
    default:
      throw new Error(`unsupported proof system: ${proofSystem}`);
  }

  const { events, transactionResult } = await verifier.execute(executeInput);
  const observedEvents: Array<{ event: string; data: unknown }> = [];
  for (const eventName of ["includedInBlock", "finalized", "error", "ErrorEvent"]) {
    events.on(eventName, (data: unknown) => {
      observedEvents.push({ data, event: eventName });
    });
  }
  const result = await transactionResult;
  console.log(JSON.stringify({ events: observedEvents, transactionResult: result }));
} finally {
  await session.close();
}
