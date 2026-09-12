import test from 'node:test';
import assert from 'node:assert/strict';
import { checkRelease } from './check-release.mjs';

test('matching published metadata passes', () => {
  assert.equal(checkRelease('v0.1.0', { version: '0.1.0' }, '## [0.1.0] - 2026-09-12\n'), '0.1.0');
});
test('rejects mismatched npm version before publication', () => {
  assert.throws(() => checkRelease('v0.2.0', { version: '0.1.0' }, ''), /does not match/);
});
test('unreleased notes cannot masquerade as a release', () => {
  assert.throws(() => checkRelease('v0.1.0', { version: '0.1.0' }, '## [Unreleased]\n'), /dated/);
});
test('rejects invalid tags and prereleases from the stable publishing path', () => {
  for (const tag of [undefined, 'main', 'v01.0.0', 'v1.0.0-rc.1', 'v1.0.0\n']) {
    assert.throws(() => checkRelease(tag, {}, ''), /stable release tag/);
  }
});
