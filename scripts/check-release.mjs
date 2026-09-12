import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

export function checkRelease(tag, pkg, changelog) {
  if (typeof tag !== 'string' || tag !== tag.trim() || !/^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/.test(tag ?? '')) {
    throw new Error('Expected a stable release tag, such as v0.1.0.');
  }
  const version = tag.slice(1);
  if (pkg.version !== version) {
    throw new Error(`Tag ${tag} does not match npm version ${pkg.version}.`);
  }
  const heading = `## [${version}] - `;
  if (!changelog.split('\n').some(line => line.startsWith(heading) && /^\d{4}-\d{2}-\d{2}$/.test(line.slice(heading.length)))) {
    throw new Error(`CHANGELOG.md needs a dated ${version} release entry.`);
  }
  return version;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const pkg = JSON.parse(readFileSync(new URL('../npm/package.json', import.meta.url)));
    const changelog = readFileSync(new URL('../CHANGELOG.md', import.meta.url), 'utf8');
    console.log(`Release ${checkRelease(process.argv[2], pkg, changelog)} metadata verified.`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
