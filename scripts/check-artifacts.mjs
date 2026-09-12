import assert from 'node:assert/strict';
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import installer from '../npm/postinstall.js';

const directory = resolve(process.argv[2] ?? 'dist');
const { version } = JSON.parse(readFileSync(join(directory, 'metadata.json')));
const checksums = readFileSync(join(directory, 'checksums.txt'));
const license = readFileSync(new URL('../LICENSE', import.meta.url));
const hostOS = process.platform === 'win32' ? 'windows' : process.platform;
const hostArch = process.arch === 'x64' ? 'amd64' : process.arch;
for (const os of ['darwin', 'linux', 'windows']) {
  for (const arch of ['amd64', 'arm64']) {
    const windows = os === 'windows';
    const name = `figctl_${version}_${os}_${arch}.${windows ? 'zip' : 'tar.gz'}`;
    const archive = readFileSync(join(directory, name));
    installer.verify(checksums, name, archive);
    const extract = windows ? installer.extractZip : installer.extractTarGz;
    assert.deepEqual(extract(archive, 'LICENSE'), license, `${name}: license`);
    for (const file of ['README.md', 'CHANGELOG.md']) {
      assert.ok(extract(archive, file)?.length, `${name}: missing ${file}`);
    }
    const binaryName = windows ? 'figctl.exe' : 'figctl';
    const binary = extract(archive, binaryName);
    assert.ok(binary?.length, `${name}: missing executable`);
    if (os === hostOS && arch === hostArch) {
      const temp = mkdtempSync(join(tmpdir(), 'figctl-artifact-'));
      try {
        const executable = join(temp, binaryName);
        writeFileSync(executable, binary, { mode: 0o755 });
        for (const args of [['version'], ['--help'], ['schema', 'envelope']]) {
          const result = spawnSync(executable, args, { encoding: 'utf8' });
          assert.equal(result.status, 0, `${args}: ${result.error ?? result.stderr}`);
          assert.ok(result.stdout.length);
          if (args[0] === 'version') assert.ok(result.stdout.includes(version));
        }
      } finally {
        rmSync(temp, { recursive: true, force: true });
      }
    }
    console.log(`Verified ${name}`);
  }
}
