// Generate only the new community API contracts from the checked-in Swagger document.
import { readFile, realpath } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const root = new URL('../', import.meta.url);
const require = createRequire(
  await realpath(
    new URL('admin/node_modules/@c-x/cx-swagger-api/package.json', root),
  ),
);
const { generateApi } = await import(require.resolve('swagger-typescript-api'));
const swagger = JSON.parse(
  await readFile(
    new URL('../../backend/docs/swagger.json', import.meta.url),
    'utf8',
  ),
);
swagger.paths = Object.fromEntries(
  Object.entries(swagger.paths).filter(([path]) =>
    path.startsWith('/api/v1/knowledge-agent/'),
  ),
);
const required = new Set();
function visit(value) {
  if (!value || typeof value !== 'object') return;
  if (value.$ref?.startsWith('#/definitions/')) {
    const name = value.$ref.slice('#/definitions/'.length);
    if (!required.has(name)) {
      required.add(name);
      visit(swagger.definitions[name]);
    }
  }
  Object.values(value).forEach(visit);
}
visit(swagger.paths);
swagger.definitions = Object.fromEntries(
  Object.entries(swagger.definitions).filter(([name]) => required.has(name)),
);
await generateApi({
  spec: swagger,
  output: fileURLToPath(new URL('admin/src/request/generated', root)),
  fileName: 'knowledgeAgent.generated.ts',
  generateClient: false,
  generateRouteTypes: true,
  silent: true,
});
