#!/usr/bin/env node

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

const extensionDir = path.join(__dirname, '..');
const projectRoot = path.join(extensionDir, '..');
const binDir = path.join(extensionDir, 'bin');
const cliDir = path.join(projectRoot, 'cmd', 'cli');

console.log('Building silky binaries for all platforms...');
console.log(`Extension dir: ${extensionDir}`);
console.log(`Project root: ${projectRoot}`);

// Create bin directory
if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
}

// Platform configurations
const platforms = [
    { goos: 'linux', goarch: 'amd64', ext: '' },
    { goos: 'linux', goarch: 'arm64', ext: '' },
    { goos: 'darwin', goarch: 'amd64', ext: '' },
    { goos: 'darwin', goarch: 'arm64', ext: '' },
    { goos: 'windows', goarch: 'amd64', ext: '.exe' },
    { goos: 'windows', goarch: 'arm64', ext: '.exe' },
];

const builtBinaries = [];

for (const platform of platforms) {
    const { goos, goarch, ext } = platform;
    const binaryName = `silky-${goos}-${goarch}${ext}`;
    const outputPath = path.join(binDir, binaryName);

    console.log(`\nBuilding for ${goos}/${goarch}...`);
    console.log(`Output: ${outputPath}`);

    try {
        execSync(`go build -o "${outputPath}" -ldflags="-s -w" .`, {
            cwd: cliDir,
            env: {
                ...process.env,
                GOOS: goos,
                GOARCH: goarch,
                CGO_ENABLED: '0', // Disable CGO for cross-platform builds
            },
            stdio: 'inherit'
        });

        const stats = fs.statSync(outputPath);
        const sizeMB = (stats.size / 1024 / 1024).toFixed(2);
        console.log(`✓ Built successfully: ${sizeMB} MB`);

        builtBinaries.push({ platform: `${goos}/${goarch}`, name: binaryName, size: sizeMB });
    } catch (error) {
        console.error(`✗ Build failed for ${goos}/${goarch}:`, error.message);
    }
}

console.log('\n=== Build Summary ===');
console.log(`Total binaries built: ${builtBinaries.length}/${platforms.length}`);
for (const binary of builtBinaries) {
    console.log(`  ✓ ${binary.platform.padEnd(15)} ${binary.name.padEnd(35)} ${binary.size} MB`);
}

// Create symlink or copy for current platform
const currentPlatform = os.platform();
const currentArch = os.arch() === 'x64' ? 'amd64' : os.arch();
const currentExt = currentPlatform === 'win32' ? '.exe' : '';
const platformBinary = `silky-${currentPlatform === 'win32' ? 'windows' : currentPlatform === 'darwin' ? 'darwin' : 'linux'}-${currentArch}${currentExt}`;
const platformBinaryPath = path.join(binDir, platformBinary);
const genericPath = path.join(binDir, `silky${currentExt}`);

if (fs.existsSync(platformBinaryPath)) {
    if (fs.existsSync(genericPath)) {
        fs.unlinkSync(genericPath);
    }

    if (currentPlatform === 'win32') {
        // On Windows, copy instead of symlink
        fs.copyFileSync(platformBinaryPath, genericPath);
        console.log(`\n✓ Created generic binary: silky${currentExt} (copy)`);
    } else {
        fs.symlinkSync(platformBinary, genericPath);
        console.log(`\n✓ Created symlink: silky → ${platformBinary}`);
    }
}

// Create a README in the bin directory
const readmePath = path.join(binDir, 'README.md');
const readmeContent = `# Silky Binaries

This directory contains pre-built binaries for different platforms.

## Available Binaries

${builtBinaries.map(b => `- **${b.platform}**: \`${b.name}\` (${b.size} MB)`).join('\n')}

## Usage

The extension automatically selects the appropriate binary for your platform.

You can also run the binary directly from the command line:

\`\`\`bash
./silky-<platform>-<arch> -config path/to/config.silky.yaml -profiler
\`\`\`

### Flags

- \`-config <path>\`: Path to configuration file (required)
- \`-profiler\`: Enable profiler output (JSON per step)
- \`-validate\`: Only validate configuration without running

## Building from Source

To rebuild binaries:

\`\`\`bash
npm run build:binary           # Build for current platform
npm run build:binary:all        # Build for all platforms
\`\`\`
`;

fs.writeFileSync(readmePath, readmeContent);
console.log(`\n✓ Created ${readmePath}`);
