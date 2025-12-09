// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';
import { StepsTreeProvider, StepTreeItem } from './stepsTreeProvider';
import { CrawlerRunner } from './crawlerRunner';
import { StepDetailsPanel } from './stepDetailsPanel';
import { TimelineViewProvider } from './timelineViewProvider';
import * as Diff from 'diff';

let crawlerRunner: CrawlerRunner | undefined;
let stepsTreeProvider: StepsTreeProvider | undefined;
let timelineViewProvider: TimelineViewProvider | undefined;

export function activate(context: vscode.ExtensionContext) {
    console.log('Silky extension is now active');

    // Configure YAML extension to recognize our files
    configureYamlExtension(context);

    // Initialize tree provider
    stepsTreeProvider = new StepsTreeProvider();
    const stepsTreeView = vscode.window.createTreeView('silky.stepsExplorer', {
        treeDataProvider: stepsTreeProvider,
        showCollapseAll: true
    });

    context.subscriptions.push(stepsTreeView);

    // Create output channel for timeline debugging
    const timelineOutputChannel = vscode.window.createOutputChannel('Silky Timeline');
    context.subscriptions.push(timelineOutputChannel);

    // Register timeline webview view provider BEFORE crawler runner
    timelineViewProvider = new TimelineViewProvider(context.extensionUri, stepsTreeProvider, timelineOutputChannel);
    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(
            TimelineViewProvider.viewType,
            timelineViewProvider
        )
    );

    // Initialize crawler runner
    crawlerRunner = new CrawlerRunner(stepsTreeProvider, timelineViewProvider, context);

    // Helper to find and focus a Silky file
    let lastSilkyFile: vscode.Uri | undefined;
    const getSilkyDocument = async (): Promise<vscode.TextDocument | undefined> => {
        // Try active editor first
        let editor = vscode.window.activeTextEditor;
        if (editor && isSilkyFile(editor.document)) {
            lastSilkyFile = editor.document.uri;
            return editor.document;
        }

        // Try to find an open Silky file in visible editors
        for (const visibleEditor of vscode.window.visibleTextEditors) {
            if (isSilkyFile(visibleEditor.document)) {
                await vscode.window.showTextDocument(visibleEditor.document, { preview: false, preserveFocus: false });
                lastSilkyFile = visibleEditor.document.uri;
                return visibleEditor.document;
            }
        }

        // Try to refocus the last used Silky file
        if (lastSilkyFile) {
            try {
                const document = await vscode.workspace.openTextDocument(lastSilkyFile);
                if (isSilkyFile(document)) {
                    await vscode.window.showTextDocument(document, { preview: false, preserveFocus: false });
                    return document;
                }
            } catch (e) {
                // File might have been closed or deleted
                lastSilkyFile = undefined;
            }
        }

        // Try to find any open Silky file in all tabs
        for (const document of vscode.workspace.textDocuments) {
            if (isSilkyFile(document)) {
                await vscode.window.showTextDocument(document, { preview: false, preserveFocus: false });
                lastSilkyFile = document.uri;
                return document;
            }
        }

        return undefined;
    };

    // Register commands
    context.subscriptions.push(
        vscode.commands.registerCommand('silky.run', async () => {
            const document = await getSilkyDocument();
            if (!document) {
                vscode.window.showWarningMessage('No Silky configuration file found. Please open an .silky.yaml file.');
                return;
            }

            await document.save();
            await crawlerRunner?.run(document.uri.fsPath);
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.stop', () => {
            crawlerRunner?.stop();
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.debug', async () => {
            const document = await getSilkyDocument();
            if (!document) {
                vscode.window.showWarningMessage('No Silky configuration file found. Please open an .silky.yaml file.');
                return;
            }

            await document.save();
            await crawlerRunner?.debug(document.uri.fsPath);
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.validateConfig', async () => {
            const document = await getSilkyDocument();
            if (!document) {
                vscode.window.showWarningMessage('No Silky configuration file found. Please open an .silky.yaml file.');
                return;
            }

            await crawlerRunner?.validate(document.uri.fsPath);
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.exportSteps', async () => {
            if (!stepsTreeProvider?.hasSteps()) {
                vscode.window.showWarningMessage('No execution steps to export');
                return;
            }

            const uri = await vscode.window.showSaveDialog({
                defaultUri: vscode.Uri.file('execution-steps.json'),
                filters: { 'JSON': ['json'] }
            });

            if (uri) {
                await stepsTreeProvider?.exportSteps(uri.fsPath);
                vscode.window.showInformationMessage(`Steps exported to ${uri.fsPath}`);
            }
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.collapseAllSteps', () => {
            vscode.commands.executeCommand('workbench.actions.treeView.silky.stepsExplorer.collapseAll');
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.refreshSteps', () => {
            stepsTreeProvider?.refresh();
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.showStepDetails', (step: StepTreeItem) => {
            if (step && step.data) {
                StepDetailsPanel.createOrShow(context.extensionUri, step.data);
            }
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.toggleStepDetails', (step: StepTreeItem) => {
            if (step && step.data) {
                StepDetailsPanel.createOrShow(context.extensionUri, step.data);
            }
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.selectStepFromTimeline', async (eventId: string) => {
            console.log('[selectStepFromTimeline] Received eventId:', eventId);

            try {
                if (stepsTreeProvider && stepsTreeView) {
                    const step = stepsTreeProvider.getStepById(eventId);
                    console.log('[selectStepFromTimeline] Found step:', step ? step.label : 'NOT FOUND');
                    console.log('[selectStepFromTimeline] Step ID:', step?.id);
                    console.log('[selectStepFromTimeline] Step data.id:', step?.data.id);

                    if (step) {
                        // First focus the Silky activity bar to make the tree visible
                        console.log('[selectStepFromTimeline] Focusing steps explorer...');
                        await vscode.commands.executeCommand('silky.stepsExplorer.focus');
                        console.log('[selectStepFromTimeline] Focus complete');

                        // Reveal and select the step in the tree with maximum visibility
                        console.log('[selectStepFromTimeline] Revealing step in tree...');
                        try {
                            await stepsTreeView.reveal(step, {
                                select: true,
                                focus: true,
                                expand: true
                            });
                            console.log('[selectStepFromTimeline] Reveal complete');
                        } catch (revealError) {
                            console.error('[selectStepFromTimeline] Reveal failed:', revealError);
                            throw revealError;
                        }

                        // Small delay to ensure the reveal completes and selection is visible
                        console.log('[selectStepFromTimeline] Waiting for UI update...');
                        await new Promise(resolve => setTimeout(resolve, 150));

                        // Trigger the same command that clicking the tree item would trigger
                        console.log('[selectStepFromTimeline] Executing toggleStepDetails...');
                        await vscode.commands.executeCommand('silky.toggleStepDetails', step);
                        console.log('[selectStepFromTimeline] Complete!');
                    } else {
                        vscode.window.showErrorMessage(`Step with ID ${eventId} not found in tree`);
                        console.error(`[selectStepFromTimeline] Step with ID ${eventId} not found in tree`);
                        console.error('[selectStepFromTimeline] Available step IDs:', Array.from(stepsTreeProvider['stepMap'].keys()));
                    }
                } else {
                    console.error('[selectStepFromTimeline] stepsTreeProvider or stepsTreeView not available');
                }
            } catch (error) {
                console.error('[selectStepFromTimeline] Fatal error:', error);
                vscode.window.showErrorMessage(`Failed to select step: ${error}`);
            }
        })
    );

    // Wire up timeline callback to steps provider (provider already registered above)
    stepsTreeProvider.setTimelineCallback((event) => {
        if (timelineViewProvider) {
            timelineViewProvider.addEvent(event);
        }
    });

    context.subscriptions.push(
        vscode.commands.registerCommand('silky.convertToSilkyYaml', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                vscode.window.showErrorMessage('No active editor');
                return;
            }

            const document = editor.document;
            const oldUri = document.uri;

            // Check if already an silky file
            if (isSilkyFile(document)) {
                vscode.window.showInformationMessage('File is already a Silky YAML file');
                return;
            }

            // Create new filename
            const oldPath = oldUri.fsPath;
            const newPath = oldPath.replace(/\.ya?ml$/, '.silky.yaml');

            if (oldPath === newPath) {
                // Doesn't have .yaml or .yml extension
                const newPathWithExt = oldPath + '.silky.yaml';
                const newUri = vscode.Uri.file(newPathWithExt);

                await vscode.workspace.fs.rename(oldUri, newUri);
                await vscode.window.showTextDocument(newUri);
                vscode.window.showInformationMessage('File renamed to Silky YAML format');
            } else {
                const newUri = vscode.Uri.file(newPath);

                await vscode.workspace.fs.rename(oldUri, newUri);
                await vscode.window.showTextDocument(newUri);
                vscode.window.showInformationMessage('File converted to Silky YAML format');
            }
        })
    );

    // Detect potential Silky files and suggest conversion
    // Also force YAML language for .silky.yaml files
    context.subscriptions.push(
        vscode.window.onDidChangeActiveTextEditor(async (editor) => {
            if (!editor) {
                return;
            }

            const document = editor.document;

            // If it's an silky file but not recognized as YAML, set it
            if (isSilkyFile(document) && document.languageId !== 'yaml') {
                await vscode.languages.setTextDocumentLanguage(document, 'yaml');
                return;
            }

            // Skip if already an silky file
            if (isSilkyFile(document)) {
                return;
            }

            // Check if it's a regular YAML file
            if (document.languageId !== 'yaml') {
                return;
            }

            // Check if content looks like Silky config
            const text = document.getText();
            if (text.includes('rootContext:') && text.includes('steps:')) {
                const action = await vscode.window.showInformationMessage(
                    'This looks like a Silky configuration. Rename to .silky.yaml to enable features?',
                    'Rename',
                    'Not now'
                );

                if (action === 'Rename') {
                    await vscode.commands.executeCommand('silky.convertToSilkyYaml');
                }
            }
        })
    );

    // Also check on document open
    context.subscriptions.push(
        vscode.workspace.onDidOpenTextDocument(async (document) => {
            if (isSilkyFile(document) && document.languageId !== 'yaml') {
                await vscode.languages.setTextDocumentLanguage(document, 'yaml');
            }
        })
    );

    // Auto-validate on save if enabled
    context.subscriptions.push(
        vscode.workspace.onDidSaveTextDocument(async (document) => {
            const config = vscode.workspace.getConfiguration('silky');
            if (config.get('autoValidate') && isSilkyFile(document)) {
                await crawlerRunner?.validate(document.uri.fsPath);
            }
        })
    );

    // Auto-run on save if enabled
    context.subscriptions.push(
        vscode.workspace.onDidSaveTextDocument(async (document) => {
            const config = vscode.workspace.getConfiguration('silky');
            if (config.get('autoRun') && isSilkyFile(document)) {
                await crawlerRunner?.run(document.uri.fsPath);
            }
        })
    );
}

export function deactivate() {
    crawlerRunner?.dispose();
}

function isSilkyFile(document: vscode.TextDocument): boolean {
    return document.languageId === 'silky-yaml' ||
           document.fileName.endsWith('.silky.yaml') ||
           document.fileName.endsWith('.silky.yml');
}

function showStepDetailsPanel(context: vscode.ExtensionContext, step: any) {
    if (!step || !step.label) {
        vscode.window.showErrorMessage('No step selected or step data is invalid');
        return;
    }

    const panel = vscode.window.createWebviewPanel(
        'silkyStepDetails',
        `Step: ${step.label}`,
        vscode.ViewColumn.Beside,
        {
            enableScripts: true,
            retainContextWhenHidden: true
        }
    );

    panel.webview.html = getStepDetailsHtml(step);
}

function generateGitLikeDiff(before: any, after: any): string {
    const beforeStr = JSON.stringify(before, null, 2);
    const afterStr = JSON.stringify(after, null, 2);

    const diff = Diff.createPatch('data', beforeStr, afterStr, '', '');
    const lines = diff.split('\n').slice(4); // Skip patch headers

    let html = '<div class="diff-view">';
    for (const line of lines) {
        if (!line) continue;

        const firstChar = line.charAt(0);
        let lineClass = '';
        let linePrefix = '';

        if (firstChar === '+') {
            lineClass = 'diff-add';
            linePrefix = '+';
        } else if (firstChar === '-') {
            lineClass = 'diff-remove';
            linePrefix = '-';
        } else if (firstChar === '@') {
            lineClass = 'diff-hunk';
            linePrefix = '';
        } else {
            lineClass = 'diff-context';
            linePrefix = ' ';
        }

        const escapedLine = escapeHtml(linePrefix + line.substring(1));
        html += `<div class="diff-line ${lineClass}">${escapedLine}</div>`;
    }
    html += '</div>';

    return html;
}

function getStepDetailsHtml(step: any): string {
    const data = step.data || {};
    const config = data.Config || {};
    const extra = data.Extra || {};

    return `<!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Step Details</title>
        <style>
            body {
                font-family: var(--vscode-font-family);
                color: var(--vscode-foreground);
                background-color: var(--vscode-editor-background);
                padding: 20px;
            }
            h1 { color: var(--vscode-textLink-foreground); margin-top: 0; }
            h2 { color: var(--vscode-textPreformat-foreground); border-bottom: 1px solid var(--vscode-panel-border); padding-bottom: 5px; margin-top: 30px; }
            .section { margin-bottom: 30px; }
            .data-view {
                background-color: var(--vscode-textCodeBlock-background);
                border: 1px solid var(--vscode-panel-border);
                border-radius: 4px;
                padding: 15px;
                overflow-x: auto;
                max-height: 500px;
                overflow-y: auto;
            }
            pre {
                margin: 0;
                white-space: pre-wrap;
                word-wrap: break-word;
                font-size: 12px;
                font-family: var(--vscode-editor-font-family, 'Menlo', 'Monaco', 'Courier New', monospace);
            }
            .meta-info {
                display: grid;
                grid-template-columns: auto 1fr;
                gap: 10px 20px;
                margin-bottom: 20px;
            }
            .meta-label {
                font-weight: bold;
                color: var(--vscode-symbolIcon-keywordForeground);
            }
            .badge {
                display: inline-block;
                padding: 2px 8px;
                border-radius: 3px;
                font-size: 11px;
                font-weight: bold;
                margin-left: 8px;
                background-color: var(--vscode-badge-background);
                color: var(--vscode-badge-foreground);
            }
            .tab-container {
                display: flex;
                gap: 10px;
                margin-bottom: 10px;
            }
            .tab {
                padding: 8px 16px;
                background-color: var(--vscode-button-secondaryBackground);
                color: var(--vscode-button-secondaryForeground);
                border: none;
                cursor: pointer;
                border-radius: 4px;
            }
            .tab.active {
                background-color: var(--vscode-button-background);
                color: var(--vscode-button-foreground);
            }
            .tab-content {
                display: none;
            }
            .tab-content.active {
                display: block;
            }
            /* Git-like diff styles */
            .diff-view {
                font-family: var(--vscode-editor-font-family, 'Menlo', 'Monaco', 'Courier New', monospace);
                font-size: 12px;
                line-height: 1.5;
            }
            .diff-line {
                padding: 2px 8px;
                margin: 0;
                white-space: pre-wrap;
                word-wrap: break-word;
            }
            .diff-add {
                background-color: var(--vscode-diffEditor-insertedTextBackground, rgba(0, 255, 0, 0.15));
                color: var(--vscode-diffEditor-insertedLineTextDecorationColor, inherit);
            }
            .diff-add::before {
                content: '';
                border-left: 3px solid var(--vscode-gitDecoration-addedResourceForeground, #81b88b);
                margin-right: 8px;
            }
            .diff-remove {
                background-color: var(--vscode-diffEditor-removedTextBackground, rgba(255, 0, 0, 0.15));
                color: var(--vscode-diffEditor-removedLineTextDecorationColor, inherit);
            }
            .diff-remove::before {
                content: '';
                border-left: 3px solid var(--vscode-gitDecoration-deletedResourceForeground, #c74e39);
                margin-right: 8px;
            }
            .diff-context {
                color: var(--vscode-editor-foreground);
            }
            .diff-hunk {
                background-color: var(--vscode-diffEditor-unchangedRegionBackground, rgba(128, 128, 128, 0.1));
                color: var(--vscode-descriptionForeground);
                font-style: italic;
            }
            /* Collapsible JSON viewer */
            details {
                margin-left: 20px;
            }
            details > summary {
                cursor: pointer;
                user-select: none;
                color: var(--vscode-textLink-foreground);
            }
            details > summary:hover {
                text-decoration: underline;
            }
        </style>
    </head>
    <body>
        <h1>${escapeHtml(data.Name || step.label)}</h1>

        <div class="section">
            <div class="meta-info">
                <span class="meta-label">Step Type:</span>
                <span>${escapeHtml(config.type || 'N/A')}</span>

                <span class="meta-label">Event Type:</span>
                <span>${escapeHtml(step.status || 'N/A')}</span>

                ${config.name ? `
                    <span class="meta-label">Config Name:</span>
                    <span>${escapeHtml(config.name)}</span>
                ` : ''}

                ${data.metadata?.durationMs !== undefined ? `
                    <span class="meta-label">Duration:</span>
                    <span>⏱ ${data.metadata.durationMs}ms</span>
                ` : ''}

                ${data.metadata?.responseSize !== undefined ? `
                    <span class="meta-label">Response Size:</span>
                    <span>📋 ${(data.metadata.responseSize / 1024).toFixed(2)} KB</span>
                ` : ''}

                ${data.metadata?.statusCode !== undefined ? `
                    <span class="meta-label">Status Code:</span>
                    <span class="badge">${data.metadata.statusCode}</span>
                ` : ''}

                ${data.metadata?.itemCount !== undefined ? `
                    <span class="meta-label">Item Count:</span>
                    <span>${data.metadata.itemCount} items</span>
                ` : ''}

                ${data.metadata?.parallel !== undefined && data.metadata.parallel ? `
                    <span class="meta-label">Execution Mode:</span>
                    <span>⚡ Parallel (${data.metadata.maxConcurrency || 'default'} workers)</span>
                ` : data.metadata?.parallel === false ? `
                    <span class="meta-label">Execution Mode:</span>
                    <span>🔁 Sequential</span>
                ` : ''}

                ${data.metadata?.mergeStrategy ? `
                    <span class="meta-label">Merge Strategy:</span>
                    <span>${escapeHtml(data.metadata.mergeStrategy)}</span>
                ` : ''}

                ${data.metadata?.mergeRule ? `
                    <span class="meta-label">Merge Rule:</span>
                    <span><code>${escapeHtml(data.metadata.mergeRule)}</code></span>
                ` : ''}

                ${data.metadata?.expression ? `
                    <span class="meta-label">Expression:</span>
                    <span><code>${escapeHtml(data.metadata.expression)}</code></span>
                ` : ''}

                ${data.threadID !== undefined ? `
                    <span class="meta-label">Thread ID:</span>
                    <span>Thread ${data.threadID}</span>
                ` : ''}
            </div>
        </div>

        ${config && Object.keys(config).length > 0 ? `
            <div class="section">
                <h2>Configuration</h2>
                <div class="data-view">
                    <pre>${escapeHtml(JSON.stringify(config, null, 2))}</pre>
                </div>
            </div>
        ` : ''}

        ${data.DataBefore || data.Data ? `
            <div class="section">
                <h2>Data</h2>
                <div class="tab-container">
                    ${data.DataBefore ? '<button class="tab active" onclick="showDataTab(\'before\')">Before</button>' : ''}
                    ${data.Data ? `<button class="tab ${!data.DataBefore ? 'active' : ''}" onclick="showDataTab(\'after\')">After</button>` : ''}
                    ${data.DataBefore && data.Data ? '<button class="tab" onclick="showDataTab(\'diff\')">Comparison</button>' : ''}
                </div>

                ${data.DataBefore ? `
                    <div id="before-content" class="tab-content active">
                        <div class="data-view">
                            <pre>${escapeHtml(JSON.stringify(data.DataBefore, null, 2))}</pre>
                        </div>
                    </div>
                ` : ''}

                ${data.Data ? `
                    <div id="after-content" class="tab-content ${!data.DataBefore ? 'active' : ''}">
                        <div class="data-view">
                            <pre>${escapeHtml(JSON.stringify(data.Data, null, 2))}</pre>
                        </div>
                    </div>
                ` : ''}

                ${data.DataBefore && data.Data ? `
                    <div id="diff-content" class="tab-content">
                        <div class="data-view">
                            ${generateGitLikeDiff(data.DataBefore, data.Data)}
                        </div>
                    </div>
                ` : ''}
            </div>
        ` : ''}

        ${data.Context && Object.keys(data.Context).length > 0 ? `
            <div class="section">
                <h2>Current Context</h2>
                <div class="data-view">
                    <pre>${escapeHtml(JSON.stringify(data.Context, null, 2))}</pre>
                </div>
            </div>
        ` : ''}

        ${step.data.snapshotRefs ? `
            <div class="section">
                <h2>Context Snapshots</h2>
                <div class="meta-info">
                    ${step.data.snapshotRefs.contextMapBeforeId ? `
                        <span class="meta-label">Context Map (Before):</span>
                        <span>${escapeHtml(step.data.snapshotRefs.contextMapBeforeId)}</span>
                    ` : ''}

                    ${step.data.snapshotRefs.contextMapAfterId ? `
                        <span class="meta-label">Context Map (After):</span>
                        <span>${escapeHtml(step.data.snapshotRefs.contextMapAfterId)}</span>
                    ` : ''}

                    ${step.data.snapshotRefs.inputSnapshotId ? `
                        <span class="meta-label">Input Snapshot:</span>
                        <span>${escapeHtml(step.data.snapshotRefs.inputSnapshotId)}</span>
                    ` : ''}

                    ${step.data.snapshotRefs.outputSnapshotId ? `
                        <span class="meta-label">Output Snapshot:</span>
                        <span>${escapeHtml(step.data.snapshotRefs.outputSnapshotId)}</span>
                    ` : ''}
                </div>
                <p style="margin-top: 10px; font-style: italic; color: var(--vscode-descriptionForeground);">
                    Snapshot IDs reference full context map states. Full snapshot data will be available in future versions.
                </p>
            </div>
        ` : ''}

        ${extra && Object.keys(extra).length > 0 ? `
            <div class="section">
                <h2>Extra Information</h2>
                <div class="data-view">
                    <pre>${escapeHtml(JSON.stringify(extra, null, 2))}</pre>
                </div>
            </div>
        ` : ''}

        <script>
            function showDataTab(tab) {
                const tabs = document.querySelectorAll('.tab');
                const contents = document.querySelectorAll('.tab-content');

                tabs.forEach(t => t.classList.remove('active'));
                contents.forEach(c => c.classList.remove('active'));

                if (tab === 'before') {
                    tabs[0].classList.add('active');
                    document.getElementById('before-content').classList.add('active');
                } else if (tab === 'after') {
                    tabs[1].classList.add('active');
                    document.getElementById('after-content').classList.add('active');
                } else if (tab === 'diff') {
                    tabs[2].classList.add('active');
                    document.getElementById('diff-content').classList.add('active');
                }
            }
        </script>
    </body>
    </html>`;
}

function escapeHtml(text: string): string {
    if (typeof text !== 'string') {
        text = String(text);
    }
    return text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

async function configureYamlExtension(context: vscode.ExtensionContext) {
    // Configure YAML extension to recognize .silky.yaml files
    const yamlConfig = vscode.workspace.getConfiguration('yaml');
    const filesConfig = vscode.workspace.getConfiguration('files');

    // Get schema URI
    const schemaUri = vscode.Uri.joinPath(context.extensionUri, 'schemas', 'silky-schema.json');

    // Configure schemas
    const schemas = yamlConfig.get<Record<string, string[]>>('schemas') || {};
    const schemaKey = schemaUri.toString();

    if (!schemas[schemaKey] || !schemas[schemaKey].includes('*.silky.yaml')) {
        schemas[schemaKey] = ['*.silky.yaml', '*.silky.yml'];
        await yamlConfig.update('schemas', schemas, vscode.ConfigurationTarget.Workspace);
    }

    // Configure file associations
    const associations = filesConfig.get<Record<string, string>>('associations') || {};

    if (associations['*.silky.yaml'] !== 'yaml') {
        associations['*.silky.yaml'] = 'yaml';
        associations['*.silky.yml'] = 'yaml';
        await filesConfig.update('associations', associations, vscode.ConfigurationTarget.Workspace);
    }

    // Notify user if YAML extension is not installed
    const yamlExtension = vscode.extensions.getExtension('redhat.vscode-yaml');
    if (!yamlExtension) {
        const action = await vscode.window.showWarningMessage(
            'Silky extension requires the YAML extension by Red Hat for syntax highlighting and validation.',
            'Install YAML Extension',
            'Later'
        );

        if (action === 'Install YAML Extension') {
            await vscode.commands.executeCommand('workbench.extensions.installExtension', 'redhat.vscode-yaml');
        }
    }
}
