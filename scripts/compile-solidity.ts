import solc from "solc";

const input = await new Response(Bun.stdin.stream()).text();
if (!input.trim()) {
  throw new Error("Solidity compiler input is required");
}

const output = solc.compile(input);
process.stdout.write(output);
