#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { readdir, readFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const webRoot = join(root, 'web');
const modulesRoot = join(webRoot, 'node_modules');
const checkIndex = process.argv.indexOf('--check');
const checkPath = checkIndex >= 0 ? process.argv[checkIndex + 1] : '';

const forbidden = /(?:^|[^A-Z])(AGPL|LGPL|GPL|SSPL|BUSL)(?:-|\b)|COMMONS CLAUSE/i;
const records = new Map();

async function scanNodeModules(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory() || entry.name === '.bin') continue;
    const full = join(directory, entry.name);
    if (entry.name.startsWith('@')) {
      await scanScope(full);
      continue;
    }
    await scanPackage(full);
  }
}

async function scanScope(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isDirectory()) await scanPackage(join(directory, entry.name));
  }
}

async function scanPackage(directory) {
  let data;
  try {
    data = JSON.parse(await readFile(join(directory, 'package.json'), 'utf8'));
  } catch (error) {
    console.error(`cannot inspect npm package at ${directory}: ${error.message}`);
    process.exit(1);
  }
  if (!data.name || !data.version) {
    console.error(`npm package metadata is incomplete at ${directory}`);
    process.exit(1);
  }
  const license = normalizeLicense(data.license ?? data.licenses);
  const key = `${data.name}@${data.version}`;
  records.set(key, { name: data.name, version: data.version, license });
  try {
    await scanNodeModules(join(directory, 'node_modules'));
  } catch (error) {
    if (error?.code !== 'ENOENT') throw error;
  }
}

function normalizeLicense(value) {
  if (typeof value === 'string' && value.trim()) return value.trim().replaceAll('|', ' OR ');
  if (Array.isArray(value)) {
    const values = value.map((item) => normalizeLicense(item?.type ?? item)).filter(Boolean);
    if (values.length) return values.join(' OR ');
  }
  if (value && typeof value === 'object' && typeof value.type === 'string') return value.type.trim();
  return 'UNKNOWN';
}

function escapeCell(value) {
  return String(value).replaceAll('|', '\\|').replaceAll('\n', ' ');
}

await scanNodeModules(modulesRoot);
const dependencies = [...records.values()].sort((left, right) =>
  `${left.name}@${left.version}`.localeCompare(`${right.name}@${right.version}`, 'en')
);
const violations = dependencies.filter((item) => item.license === 'UNKNOWN' || forbidden.test(item.license));
const lockBytes = await readFile(join(webRoot, 'package-lock.json'));
const lockSHA = createHash('sha256').update(lockBytes).digest('hex');
const lines = [
  '# Frontend Dependency Licenses',
  '',
  'Generated from the installed package graph for `web/package-lock.json`.',
  '',
  `- Lockfile SHA-256: \`${lockSHA}\``,
  `- Packages: ${dependencies.length}`,
  `- Policy violations: ${violations.length}`,
  '',
  '| Package | Version | License |',
  '| --- | --- | --- |',
  ...dependencies.map((item) => `| ${escapeCell(item.name)} | ${escapeCell(item.version)} | ${escapeCell(item.license)} |`),
  ''
];
const output = `${lines.join('\n')}\n`;

if (violations.length) {
  for (const violation of violations) {
    console.error(`license policy violation: ${violation.name}@${violation.version}: ${violation.license}`);
  }
  process.exitCode = 1;
}

if (checkPath) {
  let committed = '';
  try {
    committed = await readFile(resolve(root, checkPath), 'utf8');
  } catch {
    console.error(`frontend license inventory is missing: ${checkPath}`);
    process.exit(1);
  }
  if (committed !== output) {
    console.error(`frontend license inventory is stale: ${checkPath}`);
    process.exit(1);
  }
} else {
  process.stdout.write(output);
}
