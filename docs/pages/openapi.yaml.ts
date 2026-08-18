import specification from "../openapi.yaml?raw";

export function GET() {
  return new Response(specification, {
    headers: {
      "Content-Type": "application/yaml; charset=utf-8",
    },
  });
}
