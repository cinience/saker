#!/usr/bin/env node
// Postinstall for @saker-ai/saker.
//
// Detects the platform, finds the matching native binary from optionalDependencies,
// and copies it over the bin/saker.exe placeholder. After this runs, `saker` execs
// the native binary directly — no Node.js process stays resident.
//
// If the native package isn't present (--omit=optional), prints instructions and
// leaves the placeholder stub in place. cli-wrapper.cjs can be invoked as a fallback.
//
// Platform detection + PLATFORMS map is duplicated in cli-wrapper.cjs — keep in sync.

const { spawnSync } = require('child_process')
const {
  copyFileSync,
  linkSync,
  unlinkSync,
  chmodSync,
  readFileSync,
  writeFileSync,
  statSync,
} = require('fs')
const { arch } = require('os')
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
  // Rosetta 2: an x64 Node on Apple Silicon reports arch()==='x64'. Prefer
  // the native arm64 binary when running under translation.
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

function placeBinary(src, dest) {
  // Try hardlink first (instant, zero extra disk). Both src and dest live
  // under node_modules/ so same-filesystem is the common case.
  try {
    linkSync(src, dest)
  } catch (err) {
    if (err.code === 'EEXIST') {
      const stub = statSync(dest).size < 4096 ? readFileSync(dest) : null
      unlinkSync(dest)
      try {
        linkSync(src, dest)
      } catch {
        try {
          copyFileSync(src, dest)
        } catch (copyErr) {
          if (stub) {
            try {
              writeFileSync(dest, stub, { mode: 0o755 })
            } catch {
              // ignore
            }
          }
          throw copyErr
        }
      }
    } else if (err.code === 'EXDEV' || err.code === 'EPERM') {
      copyFileSync(src, dest)
    } else {
      throw err
    }
  }
  if (process.platform !== 'win32') {
    chmodSync(dest, 0o755)
  }
}

function main() {
  const platformKey = getPlatformKey()
  const info = PLATFORMS[platformKey]

  if (!info) {
    console.error(
      `[${WRAPPER_NAME} postinstall] Unsupported platform: ${process.platform} ${arch()}`,
    )
    console.error(`  Supported: ${Object.keys(PLATFORMS).join(', ')}`)
    return
  }

  let src
  try {
    const pkgDir = path.dirname(require.resolve(info.pkg + '/package.json'))
    src = path.join(pkgDir, info.bin)
  } catch {
    console.error(
      `[${WRAPPER_NAME} postinstall] Native package "${info.pkg}" not found.`,
    )
    if (platformKey === 'darwin-arm64' && arch() === 'x64') {
      console.error(
        '  You are running x64 Node under Rosetta 2 on Apple Silicon.',
      )
      console.error(
        '  Install arm64 Node and reinstall — e.g. via nvm:',
      )
      console.error(
        '    arch -arm64 zsh -c "nvm install --lts && npm i -g ' +
          WRAPPER_NAME +
          '"',
      )
      return
    }
    console.error(
      '  This happens with --omit=optional or when the download failed.',
    )
    console.error(
      '  The `saker` command will print instructions when invoked.',
    )
    console.error('  Fallback: node ' + path.join(__dirname, 'cli-wrapper.cjs'))
    return
  }

  const dest = path.join(__dirname, 'bin', 'saker.exe')

  try {
    placeBinary(src, dest)
  } catch (err) {
    console.error(
      `[${WRAPPER_NAME} postinstall] Failed to place binary: ${err.message}`,
    )
    console.error('  Fallback: node ' + path.join(__dirname, 'cli-wrapper.cjs'))
    process.exitCode = 1
  }
}

main()
