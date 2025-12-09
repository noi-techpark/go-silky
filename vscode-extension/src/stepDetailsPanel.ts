// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';
import { StepProfilerData, ProfileEventType } from './stepsTreeProvider';
import * as Diff from 'diff';
import { getNonce, escapeHtml, getStatusText, formatBytes, getMediaUri } from './webviewUtils';

export class StepDetailsPanel {
    private static currentPanel: StepDetailsPanel | undefined;
    private readonly panel: vscode.WebviewPanel;
    private readonly extensionUri: vscode.Uri;
    private disposables: vscode.Disposable[] = [];

    private constructor(panel: vscode.WebviewPanel, extensionUri: vscode.Uri) {
        this.panel = panel;
        this.extensionUri = extensionUri;

        // Handle messages from the webview
        this.panel.webview.onDidReceiveMessage(
            message => {
                switch (message.command) {
                    case 'copy':
                        vscode.env.clipboard.writeText(message.text);
                        vscode.window.showInformationMessage('Copied to clipboard');
                        break;
                    case 'openInTab':
                        this.openJsonInTab(message.content, message.title);
                        break;
                }
            },
            null,
            this.disposables
        );

        this.panel.onDidDispose(() => this.dispose(), null, this.disposables);
    }

    public static createOrShow(extensionUri: vscode.Uri, data: StepProfilerData) {
        const column = vscode.ViewColumn.Active;

        // If we already have a panel, show it
        if (StepDetailsPanel.currentPanel) {
            StepDetailsPanel.currentPanel.panel.reveal(column, false);
            StepDetailsPanel.currentPanel.update(data);
            return;
        }

        // Otherwise, create a new panel
        const panel = vscode.window.createWebviewPanel(
            'silkyStepDetails',
            'Step Details',
            column,
            {
                enableScripts: true,
                retainContextWhenHidden: true
            }
        );

        StepDetailsPanel.currentPanel = new StepDetailsPanel(panel, extensionUri);
        StepDetailsPanel.currentPanel.update(data);
    }

    public update(data: StepProfilerData) {
        this.panel.title = `Step Details: ${data.name || 'Unknown'}`;
        this.panel.webview.html = this.getHtmlForStep(data);
    }

    public dispose() {
        StepDetailsPanel.currentPanel = undefined;

        this.panel.dispose();

        while (this.disposables.length) {
            const disposable = this.disposables.pop();
            if (disposable) {
                disposable.dispose();
            }
        }
    }

    private async openJsonInTab(content: string, _title: string) {
        const doc = await vscode.workspace.openTextDocument({
            content: content,
            language: 'json'
        });
        await vscode.window.showTextDocument(doc, {
            viewColumn: vscode.ViewColumn.Beside,
            preview: false
        });
    }

    private getHtmlForStep(data: StepProfilerData): string {
        const nonce = getNonce();
        const styleUri = getMediaUri(this.panel.webview, this.extensionUri, 'styles', 'shared.css');

        return `<!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${this.panel.webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
            <title>Step Details</title>
            <link rel="stylesheet" type="text/css" href="${styleUri}">
            <style>
                body { padding: 20px; }
            </style>
        </head>
        <body>
            ${this.renderStepDetails(data)}
            <script nonce="${nonce}">
                const vscode = acquireVsCodeApi();

                // Wait for DOM to load
                document.addEventListener('DOMContentLoaded', function() {
                    // Setup copy buttons
                    document.querySelectorAll('.btn-copy').forEach(button => {
                        button.addEventListener('click', function() {
                            const targetId = this.getAttribute('data-target');
                            const targetElement = document.getElementById(targetId);
                            if (targetElement) {
                                const text = targetElement.textContent;
                                vscode.postMessage({
                                    command: 'copy',
                                    text: text
                                });
                            }
                        });
                    });

                    // Setup copy active view buttons (for transform/merge comparisons)
                    document.querySelectorAll('.btn-copy-active').forEach(button => {
                        button.addEventListener('click', function() {
                            const sectionId = this.getAttribute('data-section');
                            const section = document.getElementById(sectionId);
                            if (section) {
                                const activeView = section.querySelector('.comparison-view[style*="display: block"]');
                                if (activeView) {
                                    const text = activeView.textContent;
                                    vscode.postMessage({
                                        command: 'copy',
                                        text: text
                                    });
                                }
                            }
                        });
                    });

                    // Setup view switcher buttons
                    document.querySelectorAll('.btn-view').forEach(button => {
                        button.addEventListener('click', function() {
                            const viewId = this.getAttribute('data-view');
                            const selectedView = document.getElementById(viewId);
                            if (!selectedView) return;

                            // Find parent section
                            const section = selectedView.closest('.section');
                            if (!section) return;

                            // Hide all comparison views in this section
                            const views = section.querySelectorAll('.comparison-view');
                            views.forEach(view => view.style.display = 'none');

                            // Show the selected view
                            selectedView.style.display = 'block';

                            // Update button active states in this section
                            const buttons = section.querySelectorAll('.btn-view');
                            buttons.forEach(btn => btn.classList.remove('active'));
                            this.classList.add('active');
                        });
                    });

                    // Setup open in tab buttons
                    document.querySelectorAll('.btn-open-tab').forEach(button => {
                        button.addEventListener('click', function() {
                            const targetId = this.getAttribute('data-target');
                            const title = this.getAttribute('data-title') || 'JSON';
                            const targetElement = document.getElementById(targetId);
                            if (targetElement) {
                                const content = targetElement.textContent;
                                vscode.postMessage({
                                    command: 'openInTab',
                                    content: content,
                                    title: title
                                });
                            }
                        });
                    });

                    // Setup open active view in tab buttons (for transform/merge comparisons)
                    document.querySelectorAll('.btn-open-active').forEach(button => {
                        button.addEventListener('click', function() {
                            const sectionId = this.getAttribute('data-section');
                            const section = document.getElementById(sectionId);
                            if (section) {
                                const activeView = section.querySelector('.comparison-view[style*="display: block"]');
                                if (activeView) {
                                    // Determine title based on which view is active
                                    const activeButton = section.querySelector('.btn-view.active');
                                    const viewName = activeButton ? activeButton.textContent : 'Comparison';
                                    const content = activeView.textContent;
                                    vscode.postMessage({
                                        command: 'openInTab',
                                        content: content,
                                        title: viewName
                                    });
                                }
                            }
                        });
                    });
                });
            </script>
        </body>
        </html>`;
    }

    private renderStepDetails(data: StepProfilerData): string {
        switch (data.type) {
            case ProfileEventType.EVENT_ROOT_START:
                return this.renderRootStart(data);
            case ProfileEventType.EVENT_REQUEST_STEP_START:
                return this.renderRequestStep(data);
            case ProfileEventType.EVENT_FOREACH_STEP_START:
                return this.renderForEachStep(data);
            case ProfileEventType.EVENT_FORVALUES_STEP_START:
                return this.renderForValuesStep(data);
            case ProfileEventType.EVENT_CONTEXT_SELECTION:
                return this.renderContextSelection(data);
            case ProfileEventType.EVENT_PAGINATION_EVAL:
                return this.renderPaginationEval(data);
            case ProfileEventType.EVENT_URL_COMPOSITION:
                return this.renderUrlComposition(data);
            case ProfileEventType.EVENT_REQUEST_DETAILS:
                return this.renderRequestDetails(data);
            case ProfileEventType.EVENT_REQUEST_RESPONSE:
                return this.renderRequestResponse(data);
            case ProfileEventType.EVENT_RESPONSE_TRANSFORM:
                return this.renderResponseTransform(data);
            case ProfileEventType.EVENT_CONTEXT_MERGE:
                return this.renderContextMerge(data);
            case ProfileEventType.EVENT_PARALLELISM_SETUP:
                return this.renderParallelismSetup(data);
            case ProfileEventType.EVENT_ITEM_SELECTION:
                return this.renderItemSelection(data);
            case ProfileEventType.EVENT_AUTH_START:
            case ProfileEventType.EVENT_AUTH_END:
                return this.renderAuthStartEnd(data);
            case ProfileEventType.EVENT_AUTH_CACHED:
                return this.renderAuthCached(data);
            case ProfileEventType.EVENT_AUTH_LOGIN_START:
            case ProfileEventType.EVENT_AUTH_LOGIN_END:
                return this.renderAuthLogin(data);
            case ProfileEventType.EVENT_AUTH_TOKEN_EXTRACT:
                return this.renderAuthTokenExtract(data);
            case ProfileEventType.EVENT_AUTH_TOKEN_INJECT:
                return this.renderAuthTokenInject(data);
            case ProfileEventType.EVENT_RESULT:
            case ProfileEventType.EVENT_STREAM_RESULT:
                return this.renderResult(data);
            case ProfileEventType.EVENT_ERROR:
                return this.renderError(data);
            default:
                return this.renderGeneric(data);
        }
    }

    private renderRootStart(data: StepProfilerData): string {
        const contextMap = JSON.stringify(data.data?.contextMap || {}, null, 2);
        const config = JSON.stringify(data.data?.config || {}, null, 2);

        return `
            <div class="header">
                <h2>📦 Root Start</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Initial Context Map</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="root-context-map">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Context Map</summary>
                    <div class="code-block" id="root-context-map">${escapeHtml(contextMap)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Configuration</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="root-config">📋 Copy</button>
                    </div>
                </div>
                <details>
                    <summary>Crawler Configuration</summary>
                    <div class="code-block" id="root-config">${escapeHtml(config)}</div>
                </details>
            </div>
        `;
    }

    private renderRequestStep(data: StepProfilerData): string {
        const stepConfig = JSON.stringify(data.data?.stepConfig || {}, null, 2);
        const duration = data.duration ? `${data.duration}ms (${(data.duration / 1000).toFixed(3)}s)` : 'In progress...';
        const configId = `request-step-config-${data.id}`;

        return `
            <div class="header">
                <h2>🔄 Request Step: ${data.name}</h2>
                <div class="timestamp">⏱️  Started: ${new Date(data.timestamp).toLocaleString()}</div>
                ${data.duration ? `<div class="timestamp">⏱️  Duration: ${duration}</div>` : ''}
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Step Configuration</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${configId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Configuration</summary>
                    <div class="code-block" id="${configId}">${escapeHtml(stepConfig)}</div>
                </details>
            </div>
        `;
    }

    private renderForEachStep(data: StepProfilerData): string {
        const stepConfig = JSON.stringify(data.data?.stepConfig || {}, null, 2);
        const duration = data.duration ? `${data.duration}ms` : 'In progress...';
        const configId = `foreach-step-config-${data.id}`;

        return `
            <div class="header">
                <h2>🔁 ForEach Step: ${data.name}</h2>
                <div class="timestamp">⏱️  Started: ${new Date(data.timestamp).toLocaleString()}</div>
                ${data.duration ? `<div class="timestamp">⏱️  Duration: ${duration}</div>` : ''}
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Step Configuration</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${configId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Configuration</summary>
                    <div class="code-block" id="${configId}">${escapeHtml(stepConfig)}</div>
                </details>
            </div>
        `;
    }

    private renderForValuesStep(data: StepProfilerData): string {
        const stepConfig = JSON.stringify(data.data?.stepConfig || {}, null, 2);
        const values = JSON.stringify(data.data?.values || [], null, 2);
        const duration = data.duration ? `${data.duration}ms` : 'In progress...';
        const configId = `forvalues-step-config-${data.id}`;
        const valuesId = `forvalues-values-${data.id}`;

        return `
            <div class="header">
                <h2>📋 ForValues Step: ${data.name}</h2>
                <div class="timestamp">⏱️  Started: ${new Date(data.timestamp).toLocaleString()}</div>
                ${data.duration ? `<div class="timestamp">⏱️  Duration: ${duration}</div>` : ''}
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Literal Values</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${valuesId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Values to iterate</summary>
                    <div class="code-block" id="${valuesId}">${escapeHtml(values)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Step Configuration</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${configId}">📋 Copy</button>
                    </div>
                </div>
                <details>
                    <summary>Configuration</summary>
                    <div class="code-block" id="${configId}">${escapeHtml(stepConfig)}</div>
                </details>
            </div>
        `;
    }

    private renderContextSelection(data: StepProfilerData): string {
        const contextPath = data.data?.contextPath || '';
        const currentKey = data.data?.currentContextKey || '';
        const contextData = JSON.stringify(data.data?.currentContextData || {}, null, 2);
        const fullContextMap = JSON.stringify(data.data?.fullContextMap || {}, null, 2);
        const contextDataId = `context-data-${data.id}`;
        const fullContextMapId = `full-context-map-${data.id}`;

        return `
            <div class="header">
                <h2>📍 Context Selection</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">Context Path</div>
                <div class="context-path">${contextPath}</div>
            </div>

            <div class="section">
                <div class="section-title">Current Context Key</div>
                <div class="code-block">"${currentKey}"</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Current Context Data</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${contextDataId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Context Data</summary>
                    <div class="code-block" id="${contextDataId}">${escapeHtml(contextData)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Full Context Map (after selection)</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${fullContextMapId}">📋 Copy</button>
                    </div>
                </div>
                <details>
                    <summary>Click to show updated context map</summary>
                    <div class="code-block" id="${fullContextMapId}">${escapeHtml(fullContextMap)}</div>
                </details>
            </div>
        `;
    }

    private renderPaginationEval(data: StepProfilerData): string {
        const pageNumber = data.data?.pageNumber || 0;
        const paginationConfig = JSON.stringify(data.data?.paginationConfig || {}, null, 2);
        const previousResponse = JSON.stringify(data.data?.previousResponse || {}, null, 2);
        const beforeState = data.data?.previousState || {};
        const afterState = data.data?.afterState || {};
        const paginationConfigId = `pagination-config-${data.id}`;
        const previousResponseId = `previous-response-${data.id}`;

        return `
            <div class="header">
                <h2>⚙️  Pagination Evaluation (Page ${pageNumber})</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
                <div class="timestamp">📄 Page Number: ${pageNumber}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Pagination Configuration</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${paginationConfigId}">📋 Copy</button>
                    </div>
                </div>
                <details>
                    <summary>Configuration</summary>
                    <div class="code-block" id="${paginationConfigId}">${escapeHtml(paginationConfig)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Previous Response (used for extraction)</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${previousResponseId}">📋 Copy</button>
                    </div>
                </div>
                <details>
                    <summary>Click to show response body and headers</summary>
                    <div class="code-block" id="${previousResponseId}">${escapeHtml(previousResponse)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-title">State Comparison</div>
                <div class="side-by-side">
                    <div>
                        <h4>Before State</h4>
                        <div class="code-block">${escapeHtml(JSON.stringify(beforeState, null, 2))}</div>
                    </div>
                    <div>
                        <h4>After State</h4>
                        <div class="code-block">${escapeHtml(JSON.stringify(afterState, null, 2))}</div>
                    </div>
                </div>
            </div>
        `;
    }

    private renderUrlComposition(data: StepProfilerData): string {
        const urlTemplate = data.data?.urlTemplate || '';
        const templateContext = JSON.stringify(data.data?.goTemplateContext || {}, null, 2);
        const paginationState = JSON.stringify(data.data?.paginationState || {}, null, 2);
        const resultUrl = data.data?.resultUrl || '';
        const resultHeaders = JSON.stringify(data.data?.resultHeaders || {}, null, 2);
        const resultBody = JSON.stringify(data.data?.resultBody || {}, null, 2);
        const resultUrlId = `result-url-${data.id}`;

        return `
            <div class="header">
                <h2>🔗 URL Composition</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">URL Template</div>
                <div class="code-block">${escapeHtml(urlTemplate)}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Template Context</div>
                </div>
                <details>
                    <summary>Go Template Variables</summary>
                    <div class="code-block">${escapeHtml(templateContext)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Pagination State</div>
                </div>
                <details>
                    <summary>Pagination Parameters</summary>
                    <div class="code-block">${escapeHtml(paginationState)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">✅ Resulting URL</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${resultUrlId}">📋 Copy</button>
                    </div>
                </div>
                <div class="code-block" id="${resultUrlId}">${escapeHtml(resultUrl)}</div>
            </div>

            <div class="section">
                <details>
                    <summary>Resulting Headers</summary>
                    <div class="code-block">${escapeHtml(resultHeaders)}</div>
                </details>
            </div>

            <div class="section">
                <details>
                    <summary>Resulting Body</summary>
                    <div class="code-block">${escapeHtml(resultBody)}</div>
                </details>
            </div>
        `;
    }

    private renderRequestDetails(data: StepProfilerData): string {
        const method = data.data?.method || '';
        const url = data.data?.url || '';
        const curl = data.data?.curl || '';
        const headers = JSON.stringify(data.data?.headers || {}, null, 2);
        const body = JSON.stringify(data.data?.body || {}, null, 2);
        const curlId = `curl-command-${data.id}`;

        return `
            <div class="header">
                <h2>📤 Request Details</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">Method & URL</div>
                <div class="code-block">${method} ${escapeHtml(url)}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">💻 curl Command</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${curlId}">📋 Copy</button>
                    </div>
                </div>
                <div class="code-block" id="${curlId}">${escapeHtml(curl)}</div>
            </div>

            <div class="section">
                <details open>
                    <summary>Request Headers</summary>
                    <div class="code-block">${escapeHtml(headers)}</div>
                </details>
            </div>

            <div class="section">
                <details>
                    <summary>Request Body</summary>
                    <div class="code-block">${body ? escapeHtml(body) : '(none)'}</div>
                </details>
            </div>
        `;
    }

    private renderRequestResponse(data: StepProfilerData): string {
        const statusCode = data.data?.statusCode || 0;
        const statusClass = statusCode >= 200 && statusCode < 300 ? 'status-success' : 'status-error';
        const headers = JSON.stringify(data.data?.headers || {}, null, 2);
        const body = JSON.stringify(data.data?.body || {}, null, 2);
        const responseSize = data.data?.responseSize || 0;
        const duration = data.data?.durationMs || 0;

        return `
            <div class="header">
                <h2>📥 Response</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
                <div class="timestamp">⏱️  Duration: ${duration}ms</div>
            </div>

            <div class="section">
                <div class="section-title">Status Code</div>
                <span class="status-badge ${statusClass}">${statusCode} ${getStatusText(statusCode)}</span>
            </div>

            <div class="section">
                <div class="section-title">Response Info</div>
                <div>📦 Size: ${formatBytes(responseSize)}</div>
                <div>⏱️  Time: ${duration} ms</div>
            </div>

            <div class="section">
                <details>
                    <summary>Response Headers</summary>
                    <div class="code-block">${escapeHtml(headers)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Response Body</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="response-body-${data.id}">📋 Copy</button>
                        <button class="btn btn-open-tab" data-target="response-body-${data.id}" data-title="Response Body">📄 Open in Tab</button>
                    </div>
                </div>
                <details open>
                    <summary>Body Content</summary>
                    <div class="code-block" id="response-body-${data.id}">${escapeHtml(body)}</div>
                </details>
            </div>
        `;
    }

    private renderResponseTransform(data: StepProfilerData): string {
        const transformRule = data.data?.transformRule || '';
        const before = JSON.stringify(data.data?.beforeResponse || {}, null, 2);
        const after = JSON.stringify(data.data?.afterResponse || {}, null, 2);
        const diff = this.computeDiff(before, after);
        const sectionId = `transform-section-${data.id}`;
        const beforeId = `before-transform-${data.id}`;
        const afterId = `after-transform-${data.id}`;
        const diffId = `diff-transform-${data.id}`;

        return `
            <div class="header">
                <h2>⚡ Response Transform</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">Transform Rule</div>
                <div class="code-block">${escapeHtml(transformRule)}</div>
            </div>

            <div class="section" id="${sectionId}">
                <div class="section-header">
                    <div class="section-title">Comparison</div>
                    <div class="actions">
                        <button class="btn btn-copy-active" data-section="${sectionId}">📋 Copy</button>
                        <button class="btn btn-open-active" data-section="${sectionId}">📄 Open in Tab</button>
                    </div>
                </div>
                <div class="section-header">
                    <button class="btn btn-view active" data-view="${beforeId}">Before</button>
                    <button class="btn btn-view" data-view="${afterId}">After</button>
                    <button class="btn btn-view" data-view="${diffId}">Diff</button>
                </div>

                <div id="${beforeId}" class="comparison-view" style="display: block;">
                    <div class="code-block">${escapeHtml(before)}</div>
                </div>

                <div id="${afterId}" class="comparison-view" style="display: none;">
                    <div class="code-block">${escapeHtml(after)}</div>
                </div>

                <div id="${diffId}" class="comparison-view" style="display: none;">
                    <div class="diff-container">${diff}</div>
                </div>
            </div>
        `;
    }

    private renderContextMerge(data: StepProfilerData): string {
        const currentKey = data.data?.currentContextKey || '';
        const targetKey = data.data?.targetContextKey || '';
        const mergeRule = data.data?.mergeRule || '';
        const before = JSON.stringify(data.data?.targetContextBefore || {}, null, 2);
        const after = JSON.stringify(data.data?.targetContextAfter || {}, null, 2);
        const fullContextMap = JSON.stringify(data.data?.fullContextMap || {}, null, 2);
        const diff = this.computeDiff(before, after);
        const sectionId = `merge-section-${data.id}`;
        const beforeId = `before-merge-${data.id}`;
        const afterId = `after-merge-${data.id}`;
        const diffId = `diff-merge-${data.id}`;

        return `
            <div class="header">
                <h2>🔀 Context Merge</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">Merge Direction</div>
                <div>From: <span class="context-path">${currentKey}</span></div>
                <div>To: <span class="context-path">${targetKey}</span></div>
            </div>

            <div class="section">
                <div class="section-title">Merge Rule</div>
                <div class="code-block">${escapeHtml(mergeRule)}</div>
            </div>

            <div class="section" id="${sectionId}">
                <div class="section-header">
                    <div class="section-title">Target Context Comparison</div>
                    <div class="actions">
                        <button class="btn btn-copy-active" data-section="${sectionId}">📋 Copy</button>
                        <button class="btn btn-open-active" data-section="${sectionId}">📄 Open in Tab</button>
                    </div>
                </div>
                <div class="section-header">
                    <button class="btn btn-view active" data-view="${beforeId}">Before</button>
                    <button class="btn btn-view" data-view="${afterId}">After</button>
                    <button class="btn btn-view" data-view="${diffId}">Diff</button>
                </div>

                <div id="${beforeId}" class="comparison-view" style="display: block;">
                    <div class="code-block">${escapeHtml(before)}</div>
                </div>

                <div id="${afterId}" class="comparison-view" style="display: none;">
                    <div class="code-block">${escapeHtml(after)}</div>
                </div>

                <div id="${diffId}" class="comparison-view" style="display: none;">
                    <div class="diff-container">${diff}</div>
                </div>
            </div>

            <div class="section">
                <details>
                    <summary>Full Context Map (after merge)</summary>
                    <div class="code-block">${escapeHtml(fullContextMap)}</div>
                </details>
            </div>
        `;
    }

    private renderParallelismSetup(data: StepProfilerData): string {
        const maxConcurrency = data.data?.maxConcurrency || 0;
        const workerPoolId = data.data?.workerPoolId || '';
        const workerIds = data.data?.workerIds || [];
        const rateLimit = data.data?.rateLimit || null;
        const burst = data.data?.burst || 1;

        return `
            <div class="header">
                <h2>⚡ Parallelism Setup</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">Worker Pool Configuration</div>
                <div>🏊 Pool ID: <code>${workerPoolId}</code></div>
                <div>👷 Max Concurrency: <strong>${maxConcurrency}</strong> workers</div>
                <div>🔢 Worker IDs: ${workerIds.join(', ')}</div>
            </div>

            ${rateLimit ? `
            <div class="section">
                <div class="section-title">Rate Limiting</div>
                <div>🚦 Rate Limit: ${rateLimit} requests/second</div>
                <div>💥 Burst: ${burst}</div>
            </div>
            ` : ''}
        `;
    }

    private renderItemSelection(data: StepProfilerData): string {
        const iterationIndex = data.data?.iterationIndex ?? 0;
        const itemValue = JSON.stringify(data.data?.itemValue || {}, null, 2);
        const currentKey = data.data?.currentContextKey || '';
        const contextData = JSON.stringify(data.data?.currentContextData || {}, null, 2);
        const itemValueId = `item-value-${data.id}`;
        const contextDataId = `item-context-data-${data.id}`;

        return `
            <div class="header">
                <h2>📦 Item Selection</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
                ${data.workerId !== undefined ? `<div class="timestamp">👷 Worker: ${data.workerId}</div>` : ''}
            </div>

            <div class="section">
                <div class="section-title">Iteration Index</div>
                <div><strong>${iterationIndex}</strong></div>
            </div>

            <div class="section">
                <div class="section-title">Current Context Key</div>
                <div class="context-path">${currentKey}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Item Value</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${itemValueId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Value</summary>
                    <div class="code-block" id="${itemValueId}">${escapeHtml(itemValue)}</div>
                </details>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Current Context Data</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${contextDataId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Context</summary>
                    <div class="code-block" id="${contextDataId}">${escapeHtml(contextData)}</div>
                </details>
            </div>
        `;
    }

    private renderResult(data: StepProfilerData): string {
        const result = JSON.stringify(data.data?.result || data.data?.entity || {}, null, 2);
        const index = data.data?.index;
        const resultId = `result-data-${data.id}`;

        return `
            <div class="header">
                <h2>✅ ${data.type === ProfileEventType.EVENT_STREAM_RESULT ? `Stream Result ${index ?? ''}` : 'Final Result'}</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Result Data</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${resultId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Data</summary>
                    <div class="code-block" id="${resultId}">${escapeHtml(result)}</div>
                </details>
            </div>
        `;
    }

    private renderAuthStartEnd(data: StepProfilerData): string {
        const authType = data.data?.authType || 'unknown';
        const isStart = data.type === ProfileEventType.EVENT_AUTH_START;
        const duration = data.duration ? `${data.duration}ms` : '';

        return `
            <div class="header">
                <h2>🔐 ${isStart ? 'Authentication Start' : 'Authentication End'}: ${authType}</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
                ${duration ? `<div class="timestamp">⏱️  Duration: ${duration}</div>` : ''}
            </div>

            <div class="section">
                <div class="section-title">Authentication Type</div>
                <div class="code-block">${authType}</div>
            </div>

            ${data.data?.error ? `
            <div class="section">
                <div class="section-title">Error</div>
                <div class="code-block status-error">${escapeHtml(data.data.error)}</div>
            </div>
            ` : ''}

            ${data.data ? `
            <div class="section">
                <details>
                    <summary>Authentication Data</summary>
                    <div class="code-block">${escapeHtml(JSON.stringify(data.data, null, 2))}</div>
                </details>
            </div>
            ` : ''}
        `;
    }

    private renderAuthCached(data: StepProfilerData): string {
        const token = data.data?.token || '';
        const age = data.data?.age || '';
        const cookieName = data.data?.cookieName || '';
        const cookieValue = data.data?.cookieValue || '';

        return `
            <div class="header">
                <h2>💾 Cached Credentials</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            ${age ? `
            <div class="section">
                <div class="section-title">Cache Age</div>
                <div class="code-block">${age}</div>
            </div>
            ` : ''}

            ${token ? `
            <div class="section">
                <div class="section-title">Token (Masked)</div>
                <div class="code-block">${escapeHtml(token)}</div>
            </div>
            ` : ''}

            ${cookieName ? `
            <div class="section">
                <div class="section-title">Cookie Name</div>
                <div class="code-block">${escapeHtml(cookieName)}</div>
            </div>
            ` : ''}

            ${cookieValue ? `
            <div class="section">
                <div class="section-title">Cookie Value (Masked)</div>
                <div class="code-block">${escapeHtml(cookieValue)}</div>
            </div>
            ` : ''}

            <div class="info-box">
                ℹ️  Using cached authentication credentials - no new login required
            </div>
        `;
    }

    private renderAuthLogin(data: StepProfilerData): string {
        const isStart = data.type === ProfileEventType.EVENT_AUTH_LOGIN_START;
        const url = data.data?.url || '';
        const method = data.data?.method || '';
        const duration = data.duration ? `${data.duration}ms` : '';
        const statusCode = data.data?.statusCode;
        const error = data.data?.error;
        const token = data.data?.token || '';

        return `
            <div class="header">
                <h2>🔑 ${isStart ? 'Login Request' : error ? 'Login Failed' : 'Login Complete'}</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
                ${duration ? `<div class="timestamp">⏱️  Duration: ${duration}</div>` : ''}
            </div>

            ${url ? `
            <div class="section">
                <div class="section-title">Login URL</div>
                <div class="code-block">${method} ${escapeHtml(url)}</div>
            </div>
            ` : ''}

            ${statusCode ? `
            <div class="section">
                <div class="section-title">Status Code</div>
                <span class="status-badge ${statusCode >= 200 && statusCode < 300 ? 'status-success' : 'status-error'}">${statusCode}</span>
            </div>
            ` : ''}

            ${error ? `
            <div class="section">
                <div class="section-title">Error</div>
                <div class="code-block status-error">${escapeHtml(error)}</div>
            </div>
            ` : ''}

            ${token ? `
            <div class="section">
                <div class="section-title">Token (Masked)</div>
                <div class="code-block">${escapeHtml(token)}</div>
            </div>
            ` : ''}

            ${data.data ? `
            <div class="section">
                <details>
                    <summary>Full Login Data</summary>
                    <div class="code-block">${escapeHtml(JSON.stringify(data.data, null, 2))}</div>
                </details>
            </div>
            ` : ''}
        `;
    }

    private renderAuthTokenExtract(data: StepProfilerData): string {
        const extractFrom = data.data?.extractFrom || data.data?.extractSelector || '';
        const token = data.data?.token || '';
        const cookieName = data.data?.cookieName || '';
        const cookieValue = data.data?.cookieValue || '';
        const headerName = data.data?.headerName || '';
        const jqSelector = data.data?.jqSelector || '';

        return `
            <div class="header">
                <h2>🔍 Token Extraction</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            ${extractFrom ? `
            <div class="section">
                <div class="section-title">Extract From</div>
                <div class="code-block">${escapeHtml(extractFrom)}</div>
            </div>
            ` : ''}

            ${cookieName ? `
            <div class="section">
                <div class="section-title">Cookie Name</div>
                <div class="code-block">${escapeHtml(cookieName)}</div>
            </div>
            ` : ''}

            ${cookieValue ? `
            <div class="section">
                <div class="section-title">Cookie Value (Masked)</div>
                <div class="code-block">${escapeHtml(cookieValue)}</div>
            </div>
            ` : ''}

            ${headerName ? `
            <div class="section">
                <div class="section-title">Header Name</div>
                <div class="code-block">${escapeHtml(headerName)}</div>
            </div>
            ` : ''}

            ${jqSelector ? `
            <div class="section">
                <div class="section-title">JQ Selector</div>
                <div class="code-block">${escapeHtml(jqSelector)}</div>
            </div>
            ` : ''}

            ${token ? `
            <div class="section">
                <div class="section-title">Extracted Token (Masked)</div>
                <div class="code-block">${escapeHtml(token)}</div>
            </div>
            ` : ''}

            <div class="info-box">
                ℹ️  Token successfully extracted from login response
            </div>
        `;
    }

    private renderAuthTokenInject(data: StepProfilerData): string {
        const location = data.data?.location || '';
        const format = data.data?.format || '';
        const token = data.data?.token || '';
        const headerKey = data.data?.headerKey || '';
        const queryKey = data.data?.queryKey || '';
        const cookieName = data.data?.cookieName || '';
        const cookieValue = data.data?.cookieValue || '';

        return `
            <div class="header">
                <h2>➡️  Token Injection</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-title">Injection Location</div>
                <div class="code-block">${escapeHtml(location)}</div>
            </div>

            ${format ? `
            <div class="section">
                <div class="section-title">Format</div>
                <div class="code-block">${escapeHtml(format)}</div>
            </div>
            ` : ''}

            ${token ? `
            <div class="section">
                <div class="section-title">Token (Masked)</div>
                <div class="code-block">${escapeHtml(token)}</div>
            </div>
            ` : ''}

            ${headerKey ? `
            <div class="section">
                <div class="section-title">Header Key</div>
                <div class="code-block">${escapeHtml(headerKey)}</div>
            </div>
            ` : ''}

            ${queryKey ? `
            <div class="section">
                <div class="section-title">Query Parameter Key</div>
                <div class="code-block">${escapeHtml(queryKey)}</div>
            </div>
            ` : ''}

            ${cookieName ? `
            <div class="section">
                <div class="section-title">Cookie Name</div>
                <div class="code-block">${escapeHtml(cookieName)}</div>
            </div>
            ` : ''}

            ${cookieValue ? `
            <div class="section">
                <div class="section-title">Cookie Value (Masked)</div>
                <div class="code-block">${escapeHtml(cookieValue)}</div>
            </div>
            ` : ''}

            <div class="info-box">
                ℹ️  Credentials injected into request - ready to send authenticated request
            </div>
        `;
    }

    private renderError(data: StepProfilerData): string {
        const error = data.data?.error || data.data?.message || 'Unknown error';
        const errorType = data.data?.errorType || data.data?.type || 'Error';
        const stackTrace = data.data?.stackTrace || data.data?.stack || '';
        const stepName = data.data?.stepName || data.name || '';
        const errorDetails = data.data?.details || '';
        const duration = data.duration ? `${data.duration}ms` : '';
        const errorId = `error-message-${data.id}`;
        const stackId = `error-stack-${data.id}`;
        const dataId = `error-data-${data.id}`;

        return `
            <div class="header">
                <h2>❌ ${escapeHtml(errorType)}</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
                ${duration ? `<div class="timestamp">⏱️  Duration: ${duration}</div>` : ''}
                ${stepName ? `<div class="timestamp">📍 Step: ${escapeHtml(stepName)}</div>` : ''}
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Error Message</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${errorId}">📋 Copy</button>
                    </div>
                </div>
                <div class="code-block status-error" id="${errorId}">${escapeHtml(error)}</div>
            </div>

            ${errorDetails ? `
            <div class="section">
                <div class="section-title">Error Details</div>
                <div class="code-block">${escapeHtml(errorDetails)}</div>
            </div>
            ` : ''}

            ${stackTrace ? `
            <div class="section">
                <div class="section-header">
                    <div class="section-title">Stack Trace</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${stackId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Stack Trace</summary>
                    <div class="code-block" id="${stackId}">${escapeHtml(stackTrace)}</div>
                </details>
            </div>
            ` : ''}

            ${data.data ? `
            <div class="section">
                <div class="section-header">
                    <div class="section-title">Full Error Data</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${dataId}">📋 Copy</button>
                        <button class="btn btn-open-tab" data-target="${dataId}" data-title="Error Data">📄 Open in Tab</button>
                    </div>
                </div>
                <details>
                    <summary>Complete Error Information</summary>
                    <div class="code-block" id="${dataId}">${escapeHtml(JSON.stringify(data.data, null, 2))}</div>
                </details>
            </div>
            ` : ''}
        `;
    }

    private renderGeneric(data: StepProfilerData): string {
        const dataStr = JSON.stringify(data.data || {}, null, 2);
        const dataId = `generic-data-${data.id}`;

        return `
            <div class="header">
                <h2>${data.name || 'Step'}</h2>
                <div class="timestamp">⏱️  ${new Date(data.timestamp).toLocaleString()}</div>
            </div>

            <div class="section">
                <div class="section-header">
                    <div class="section-title">Data</div>
                    <div class="actions">
                        <button class="btn btn-copy" data-target="${dataId}">📋 Copy</button>
                    </div>
                </div>
                <details open>
                    <summary>Details</summary>
                    <div class="code-block" id="${dataId}">${escapeHtml(dataStr)}</div>
                </details>
            </div>
        `;
    }

    private computeDiff(before: string, after: string): string {
        // Use jsdiff for word-level diff
        const changes = Diff.diffWords(before, after);

        let html = '<div class="diff-view">';

        for (const change of changes) {
            if (change.added) {
                // Split into lines and add each line with + prefix
                const lines = change.value.split('\n');
                for (const line of lines) {
                    if (line) {  // Skip empty lines at the end
                        html += `<div class="diff-line diff-added">+ ${escapeHtml(line)}</div>`;
                    }
                }
            } else if (change.removed) {
                // Split into lines and add each line with - prefix
                const lines = change.value.split('\n');
                for (const line of lines) {
                    if (line) {  // Skip empty lines at the end
                        html += `<div class="diff-line diff-removed">- ${escapeHtml(line)}</div>`;
                    }
                }
            } else {
                // Unchanged - split into lines
                const lines = change.value.split('\n');
                for (const line of lines) {
                    if (line || lines.length === 1) {  // Show empty lines in the middle
                        html += `<div class="diff-line diff-unchanged">  ${escapeHtml(line)}</div>`;
                    }
                }
            }
        }

        html += '</div>';
        return html;
    }
}
