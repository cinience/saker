#!/usr/bin/env node
// Fallback launcher for @saker-ai/saker.
//
// Normally the postinstall script copies the native binary over bin/saker.exe,
// so this file is never invoked. It exists for environments where postinstall
// doesn't run (--ignore-scripts) — users can run `node cli-wrapper.cjs` directly
// and pay the Node-process overhead as the price.
//
// Platform detection + PLATFORMS map is duplicated in install.cjs — keep in sync.

const { spawnSync } = require('child_process')
const { arch, constants } = require('os')
const path = require('path')

const PACKAGE_PREFIX = '@saker-ai/saker'
const BINARY_NAME = 'saker'
const WRAPPER_NAME = require('./package.json').name

const PLATFORMS = {
  'darwin-arm64': { pkg: PACKAGE_PREFIX + '-darwin-arm64', bin: BINARY_NAME },
  'darwin-x64': { pkg: PACKAGE_PREFIX + '-darwin-x64', bin: BINARY_NAME },
  'linux-x64': { pkg: PACKAGE_PREFIX + '-linux-x64', bin: BINARY_NAME },
  'linux-arm64': { pkg: PACKAGE_PREFIX + '-linux-arm64', bin: BINARY_NAME },
  'win32-x64': {
    pkg: PACKAGE_PREFIX + '-win32-x64',
    bin: BINARY_NAME + '.exe',
  },
  'win32-arm64': {
    pkg: PACKAGE_PREFIX + '-win32-arm64',
    bin: BINARY_NAME + '.exe',
  },
}

function getPlatformKey() {
  const platform = process.platform
  let cpu = arch()
  if (platform === 'darwin' && cpu === 'x64') {
    const r = spawnSync('sysctl', ['-n', 'sysctl.proc_translated'], {
      encoding: 'utf8',
    })
    if (r.stdout?.trim() === '1') {
      cpu = 'arm64'
    }
  }
  return platform + '-' + cpu
}

function getBinaryPath() {
  const platformKey = getPlatformKey()
  const info = PLATFORMS[platformKey]
  if (!info) {
    console.error(
      `[${WRAPPER_NAME}] Unsupported platform: ${process.platform} ${arch()}. Supported: ${Object.keys(PLATFORMS).join(', ')}`,
    )
    process.exit(1)
  }
  try {
    const pkgDir = path.dirname(require.resolve(info.pkg + '/package.json'))
    return path.join(pkgDir, info.bin)
  } catch {
    console.error(
      `[${WRAPPER_NAME}] Could not find native binary package "${info.pkg}".`,
    )
    if (platformKey === 'darwin-arm64' && arch() === 'x64') {
      console.error(
        '  You are running x64 Node under Rosetta 2 on Apple Silicon.',
      )
      console.error('  Install arm64 Node and reinstall.')
    } else {
      console.error('  Try reinstalling with: npm install -g @saker-ai/saker')
    }
    process.exit(1)
  }
}

function main() {
  const binaryPath = getBinaryPath()
  const result = spawnSync(binaryPath, process.argv.slice(2), {
    stdio: 'inherit',
    env: { ...process.env, SAKER_INSTALLED_VIA_NPM: '1' },
  })
  if (result.error) {
    console.error(
      `[${WRAPPER_NAME}] Failed to execute native binary at ` + binaryPath,
    )
    console.error('  ' + result.error.message)
    process.exit(1)
  }
  if (result.signal) {
    const signum = constants.signals[result.signal] ?? 0
    process.exit(128 + signum)
  } else {
    process.exit(result.status ?? 1)
  }
}

main()
