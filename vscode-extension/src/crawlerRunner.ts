// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';
import * as cp from 'child_process';
import * as path from 'path';
import { StepsTreeProvider, StepProfilerData, ProfileEventType } from './stepsTreeProvider';
import { TimelineViewProvider } from './timelineViewProvider';

export class CrawlerRunner {
    private outputChannel: vscode.OutputChannel;
    private currentProcess: cp.ChildProcess | undefined;
    private diagnosticCollection: vscode.DiagnosticCollection;

    constructor(
        private stepsProvider: StepsTreeProvider,
        private timelineProvider: TimelineViewProvider,
        private context: vscode.ExtensionContext
    ) {
        this.outputChannel = vscode.window.createOutputChannel('Silky');
        this.diagnosticCollection = vscode.languages.createDiagnosticCollection('silky');
        this.context.subscriptions.push(this.outputChannel);
        this.context.subscriptions.push(this.diagnosticCollection);
    }

    async run(configPath: string): Promise<void> {
        this.stop();
        this.stepsProvider.clear();
        this.timelineProvider.clear();
        this.outputChannel.clear();
        this.outputChannel.show(true);

        const config = vscode.workspace.getConfiguration('silky');
        const goPath = config.get<string>('executable.path') || 'go';
        const maxOutputSize = config.get<number>('maxOutputSize') || 10000;

        // Find the Go module root (where cmd/ide is located)
        const idePath = await this.findIdePath(configPath);

        this.outputChannel.appendLine(`Running Silky on: ${configPath}`);
        this.outputChannel.appendLine(`Using Go executable: ${goPath}`);
        this.outputChannel.appendLine('---');

        try {
            await vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Running Silky',
                cancellable: true
            }, async (_progress, token) => {
                token.onCancellationRequested(() => {
                    this.stop();
                });

                return new Promise<void>((resolve, reject) => {
                    // Determine if we're using bundled binary or go run
                    const usingBundled = idePath.includes('/bin') && idePath.includes(this.context.extensionPath);

                    let command: string;
                    let args: string[];
                    let cwd: string;

                    if (usingBundled) {
                        // Using bundled binary
                        const binaryPath = path.join(idePath, 'silky');
                        command = binaryPath;
                        args = ['-config', configPath, '-profiler'];
                        cwd = path.dirname(configPath);
                        this.outputChannel.appendLine(`Using bundled binary: ${binaryPath}`);
                    } else {
                        // Using go run
                        command = goPath;
                        args = ['run', '.', '-config', configPath, '-profiler'];
                        cwd = idePath;
                        this.outputChannel.appendLine(`Using go run in: ${idePath}`);
                    }

                    this.currentProcess = cp.spawn(command, args, {
                        cwd,
                        env: { ...process.env }
                    });

                    let outputLines = 0;
                    let stdoutBuffer = '';
                    let stderrBuffer = '';

                    this.currentProcess.stdout?.on('data', (data: Buffer) => {
                        const text = data.toString();
                        stdoutBuffer += text;

                        // Process complete lines
                        const lines = stdoutBuffer.split('\n');
                        stdoutBuffer = lines.pop() || ''; // Keep incomplete line in buffer

                        for (const line of lines) {
                            if (outputLines < maxOutputSize || maxOutputSize === 0) {
                                // Try to parse as profiler data
                                if (this.tryParseProfilerData(line)) {
                                    // Successfully parsed as profiler data
                                    continue;
                                }

                                // Regular output
                                this.outputChannel.appendLine(line);
                                outputLines++;
                            } else if (outputLines === maxOutputSize) {
                                this.outputChannel.appendLine('--- Output limit reached ---');
                                outputLines++;
                            }
                        }
                    });

                    this.currentProcess.stderr?.on('data', (data: Buffer) => {
                        stderrBuffer += data.toString();

                        const lines = stderrBuffer.split('\n');
                        stderrBuffer = lines.pop() || '';

                        for (const line of lines) {
                            this.outputChannel.appendLine(`[ERROR] ${line}`);
                        }
                    });

                    this.currentProcess.on('error', (error) => {
                        vscode.window.showErrorMessage(`Failed to run crawler: ${error.message}`);
                        this.outputChannel.appendLine(`Process error: ${error.message}`);
                        reject(error);
                    });

                    this.currentProcess.on('close', (code) => {
                        // Flush remaining buffers
                        if (stdoutBuffer) {
                            this.outputChannel.appendLine(stdoutBuffer);
                        }
                        if (stderrBuffer) {
                            this.outputChannel.appendLine(`[ERROR] ${stderrBuffer}`);
                        }

                        if (code === 0) {
                            this.outputChannel.appendLine('---');
                            this.outputChannel.appendLine('Execution completed successfully');
                            vscode.window.showInformationMessage('Silky execution completed');

                            // Focus timeline at the end
                            vscode.commands.executeCommand('silky.timeline.focus');

                            resolve();
                        } else {
                            this.outputChannel.appendLine('---');
                            this.outputChannel.appendLine(`Execution failed with code ${code}`);
                            vscode.window.showErrorMessage(`Silky execution failed with code ${code}`);

                            // Focus timeline even on failure
                            vscode.commands.executeCommand('silky.timeline.focus');

                            reject(new Error(`Process exited with code ${code}`));
                        }

                        this.currentProcess = undefined;
                    });
                });
            });
        } catch (error: any) {
            this.outputChannel.appendLine(`Error: ${error.message}`);
        }
    }

    async debug(configPath: string): Promise<void> {
        // Debug mode runs with profiler enabled and shows more detailed output
        this.outputChannel.appendLine(`Debugging Silky on: ${configPath}`);
        this.outputChannel.appendLine('Debug mode: Profiler enabled, verbose output');
        this.outputChannel.appendLine('---');

        // Same as run but with profiler always enabled
        await this.run(configPath);
    }

    async validate(configPath: string): Promise<void> {
        this.outputChannel.show(true);
        this.diagnosticCollection.clear();

        const config = vscode.workspace.getConfiguration('silky');
        const goPath = config.get<string>('executable.path') || 'go';

        const idePath = await this.findIdePath(configPath);

        this.outputChannel.appendLine(`Validating: ${configPath}`);

        // Determine if we're using bundled binary or go run
        const usingBundled = idePath.includes('/bin') && idePath.includes(this.context.extensionPath);

        try {
            const result = await new Promise<string>((resolve, reject) => {
                let command: string;
                let args: string[];
                let cwd: string;

                if (usingBundled) {
                    // Using bundled binary
                    const binaryPath = path.join(idePath, 'silky');
                    command = binaryPath;
                    args = ['-config', configPath, '-validate'];
                    cwd = path.dirname(configPath);
                    this.outputChannel.appendLine(`Using bundled binary: ${binaryPath}`);
                } else {
                    // Using go run
                    command = goPath;
                    args = ['run', '.', '-config', configPath, '-validate'];
                    cwd = idePath;
                    this.outputChannel.appendLine(`Using go run in: ${idePath}`);
                }

                cp.execFile(command, args, { cwd }, (error, stdout, stderr) => {
                    if (error && error.code !== 0) {
                        reject(new Error(stderr || stdout || error.message));
                    } else {
                        resolve(stdout);
                    }
                });
            });

            this.outputChannel.appendLine(result);
            vscode.window.showInformationMessage('Configuration is valid');
        } catch (error: any) {
            this.outputChannel.appendLine(`Validation failed: ${error.message}`);

            // Try to parse validation errors and add diagnostics
            await this.parseValidationErrors(configPath, error.message);

            vscode.window.showErrorMessage('Validation failed - check Problems panel for details');
        }
    }

    stop(): void {
        if (this.currentProcess) {
            this.currentProcess.kill();
            this.currentProcess = undefined;
            this.outputChannel.appendLine('Execution stopped by user');

            // Focus timeline when stopped
            vscode.commands.executeCommand('silky.timeline.focus');
        }
    }

    dispose(): void {
        this.stop();
        this.outputChannel.dispose();
        this.diagnosticCollection.dispose();
    }

    private tryParseProfilerData(line: string): boolean {
        const trimmed = line.trim();

        // Handle RESULT: prefix
        if (trimmed.startsWith('RESULT:')) {
            const jsonStr = trimmed.substring(7).trim();
        }

        // Handle STREAM: prefix
        if (trimmed.startsWith('STREAM:')) {
            const jsonStr = trimmed.substring(7).trim();
        }

        // Handle profiler data (plain JSON)
        if (!trimmed.startsWith('{')) {
            return false;
        }

        try {
            const data = JSON.parse(trimmed) as StepProfilerData;

            // Verify it looks like profiler data by checking for type field
            // ProfileEventType range is 0-17 (18 total event types)
            if (data.type !== undefined && data.type >= 0 && data.type <= ProfileEventType.EVENT_MAX_NUM) {
                this.outputChannel.appendLine(`[DEBUG] Adding step: type=${data.type}, name=${data.name}`);
                this.stepsProvider.addStep(data);
                const rootSteps = this.stepsProvider.getSteps();
                this.outputChannel.appendLine(`[DEBUG] After addStep: rootSteps.length=${rootSteps.length}, hasSteps=${this.stepsProvider.hasSteps()}`);
                if (rootSteps.length > 0) {
                    this.outputChannel.appendLine(`[DEBUG] First root step: ${rootSteps[0].label}, children=${rootSteps[0].children.length}`);
                }
                return true;
            } else {
                this.outputChannel.appendLine(`[DEBUG] Rejected event: type=${data.type}`);
            }
        } catch (e) {
            // Not valid JSON or not profiler data
            this.outputChannel.appendLine(`[DEBUG] JSON parse failed: ${e}`);
        }

        return false;
    }

    private async parseValidationErrors(configPath: string, errorMessage: string): Promise<void> {
        const diagnostics: vscode.Diagnostic[] = [];
        const uri = vscode.Uri.file(configPath);

        // Parse errors in format: "  - steps[0].request: request step requires a request field"
        const errorPattern = /^\s*-\s+([^:]+):\s+(.+)$/gm;
        let match;

        // Read the YAML file to find line numbers
        const fs = require('fs').promises;
        let yamlContent = '';
        try {
            yamlContent = await fs.readFile(configPath, 'utf8');
        } catch {
            // If we can't read the file, still show errors at line 0
        }

        const lines = yamlContent.split('\n');

        while ((match = errorPattern.exec(errorMessage)) !== null) {
            const location = match[1].trim();
            const message = match[2].trim();

            // Try to find the line number for this error location
            const lineNumber = this.findErrorLine(lines, location);

            const diagnostic = new vscode.Diagnostic(
                new vscode.Range(lineNumber, 0, lineNumber, 1000),
                `${location}: ${message}`,
                vscode.DiagnosticSeverity.Error
            );

            diagnostic.source = 'Silky';
            diagnostics.push(diagnostic);
        }

        // Also check for generic error format
        if (diagnostics.length === 0 && errorMessage.includes('validation failed')) {
            // Show a general error at the top of the file
            const diagnostic = new vscode.Diagnostic(
                new vscode.Range(0, 0, 0, 1000),
                errorMessage.trim(),
                vscode.DiagnosticSeverity.Error
            );
            diagnostic.source = 'Silky';
            diagnostics.push(diagnostic);
        }

        if (diagnostics.length > 0) {
            this.diagnosticCollection.set(uri, diagnostics);

            // Also show in Problems panel by opening it
            vscode.commands.executeCommand('workbench.action.problems.focus');
        }
    }

    private findErrorLine(lines: string[], location: string): number {
        // Parse location like "steps[0].request" or "steps[1].forEach"
        // Try to find the corresponding line in the YAML

        // Extract the path components
        const pathMatch = location.match(/steps\[(\d+)\]\.?(.+)?/);
        if (!pathMatch) {
            return 0;
        }

        const stepIndex = parseInt(pathMatch[1], 10);
        const subPath = pathMatch[2];

        // Find the steps array
        let currentStepIndex = -1;
        let inSteps = false;
        let stepIndent = 0;

        for (let i = 0; i < lines.length; i++) {
            const line = lines[i];
            const trimmed = line.trim();

            // Check if we're entering the steps array
            if (trimmed.startsWith('steps:')) {
                inSteps = true;
                stepIndent = line.search(/\S/);
                continue;
            }

            if (inSteps) {
                const lineIndent = line.search(/\S/);

                // Check if we've left the steps array
                if (lineIndent !== -1 && lineIndent <= stepIndent && !line.match(/^\s*-/)) {
                    break;
                }

                // Check if this is a step item
                if (trimmed.startsWith('- type:') || (trimmed.startsWith('-') && lines[i + 1]?.trim().startsWith('type:'))) {
                    currentStepIndex++;
                    if (currentStepIndex === stepIndex) {
                        // Found the right step
                        if (!subPath) {
                            return i;
                        }

                        // Look for the subPath within this step
                        const subPathKey = subPath.split('.')[0];
                        for (let j = i; j < lines.length; j++) {
                            const subLine = lines[j];
                            const subIndent = subLine.search(/\S/);

                            // Stop if we've left this step
                            if (subIndent !== -1 && subIndent <= lineIndent && j > i) {
                                break;
                            }

                            // Check if this line contains the subPath key
                            if (subLine.trim().startsWith(`${subPathKey}:`)) {
                                return j;
                            }
                        }

                        // If we didn't find the subPath, return the step line
                        return i;
                    }
                }
            }
        }

        return 0; // Default to first line
    }

    private async findIdePath(configPath: string): Promise<string> {
        const fs = require('fs').promises;

        // Strategy 1: Check if bundled binary exists
        const bundledBinary = path.join(this.context.extensionPath, 'bin', 'silky');
        try {
            await fs.access(bundledBinary);
            this.outputChannel.appendLine(`Using bundled binary: ${bundledBinary}`);
            return path.dirname(bundledBinary);
        } catch {
            // No bundled binary, continue to other strategies
        }

        // Strategy 2: Check workspace folders for cmd/ide
        const workspaceFolders = vscode.workspace.workspaceFolders;
        if (workspaceFolders) {
            for (const folder of workspaceFolders) {
                const idePath = path.join(folder.uri.fsPath, 'cmd', 'ide');
                try {
                    await fs.access(idePath);
                    this.outputChannel.appendLine(`Found cmd/ide in workspace: ${idePath}`);
                    return idePath;
                } catch {
                    // Not in this folder, continue
                }
            }

            // Strategy 3: Check parent directory of workspace folder
            const firstFolder = workspaceFolders[0].uri.fsPath;
            const parentPath = path.join(firstFolder, '..', 'cmd', 'ide');
            try {
                await fs.access(parentPath);
                this.outputChannel.appendLine(`Found cmd/ide in parent: ${parentPath}`);
                return path.resolve(parentPath);
            } catch {
                // Not in parent either
            }
        }

        // Strategy 4: Check relative to config file
        const configDir = path.dirname(configPath);
        const searchPaths = [
            path.join(configDir, 'cmd', 'ide'),
            path.join(configDir, '..', 'cmd', 'ide'),
            path.join(configDir, '..', '..', 'cmd', 'ide'),
        ];

        for (const searchPath of searchPaths) {
            try {
                await fs.access(searchPath);
                this.outputChannel.appendLine(`Found cmd/ide relative to config: ${searchPath}`);
                return path.resolve(searchPath);
            } catch {
                // Not here, continue
            }
        }

        // Strategy 5: Ask user to locate it
        const action = await vscode.window.showErrorMessage(
            'Could not find cmd/ide folder or bundled binary. Please select the go-silky project folder.',
            'Browse...',
            'Cancel'
        );

        if (action === 'Browse...') {
            const selected = await vscode.window.showOpenDialog({
                canSelectFiles: false,
                canSelectFolders: true,
                canSelectMany: false,
                title: 'Select go-silky project folder'
            });

            if (selected && selected[0]) {
                const idePath = path.join(selected[0].fsPath, 'cmd', 'ide');
                try {
                    await fs.access(idePath);
                    // Save for future use
                    const config = vscode.workspace.getConfiguration('silky');
                    await config.update('projectPath', selected[0].fsPath, vscode.ConfigurationTarget.Workspace);
                    return idePath;
                } catch {
                    vscode.window.showErrorMessage(`Selected folder does not contain cmd/ide: ${idePath}`);
                }
            }
        }

        throw new Error('Could not find cmd/ide folder. Please open the go-silky project folder in VSCode.');
    }
}
