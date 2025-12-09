// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';
import { StepProfilerData, ProfileEventType } from './stepsTreeProvider';
import { getNonce, escapeHtml, getMediaUri } from './webviewUtils';

// Store events globally so they persist across panel lifecycles
let globalEvents: StepProfilerData[] = [];

export class TimelinePanel {
    private static instance: TimelinePanel | undefined;
    private readonly panel: vscode.WebviewPanel;
    private readonly extensionUri: vscode.Uri;
    private disposables: vscode.Disposable[] = [];

    private constructor(panel: vscode.WebviewPanel, extensionUri: vscode.Uri) {
        this.panel = panel;
        this.extensionUri = extensionUri;

        this.panel.onDidDispose(() => this.dispose(), null, this.disposables);

        this.panel.webview.onDidReceiveMessage(
            message => {
                switch (message.command) {
                    case 'selectEvent':
                        vscode.commands.executeCommand('silky.showStepDetails', message.event);
                        break;
                }
            },
            null,
            this.disposables
        );
    }

    public static createOrShow(extensionUri: vscode.Uri) {
        // If we already have a panel, show it and refresh
        if (TimelinePanel.instance) {
            TimelinePanel.instance.panel.reveal(vscode.ViewColumn.Two, false);
            TimelinePanel.instance.refresh();
            return TimelinePanel.instance;
        }

        // Otherwise, create a new panel beside the editor
        const panel = vscode.window.createWebviewPanel(
            'silkyTimeline',
            'Execution Timeline',
            vscode.ViewColumn.Two,
            {
                enableScripts: true,
                retainContextWhenHidden: true
            }
        );

        TimelinePanel.instance = new TimelinePanel(panel, extensionUri);
        return TimelinePanel.instance;
    }

    public static getInstance(): TimelinePanel | undefined {
        return TimelinePanel.instance;
    }

    public addEvent(event: StepProfilerData) {
        globalEvents.push(event);
        this.refresh();
    }

    public clear() {
        globalEvents = [];
        this.refresh();
    }

    private refresh() {
        this.panel.webview.html = this.getHtmlContent();
    }

    public dispose() {
        TimelinePanel.instance = undefined;

        this.panel.dispose();

        while (this.disposables.length) {
            const disposable = this.disposables.pop();
            if (disposable) {
                disposable.dispose();
            }
        }
    }

    private getHtmlContent(): string {
        const nonce = getNonce();
        const styleUri = getMediaUri(this.panel.webview, this.extensionUri, 'styles', 'timeline.css');

        return `<!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${this.panel.webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
            <title>Execution Timeline</title>
            <link rel="stylesheet" type="text/css" href="${styleUri}">
            <style>
                body { padding: 20px; }
                #timeline { min-height: 400px; max-height: 80vh; }
            </style>
        </head>
        <body>
            <div class="header">
                <h3>Execution Timeline</h3>
                <div class="controls">
                    <button class="btn" onclick="zoomIn()">🔍 Zoom In</button>
                    <button class="btn" onclick="zoomOut()">🔍 Zoom Out</button>
                    <button class="btn" onclick="resetZoom()">↺ Reset</button>
                </div>
            </div>

            <div id="timeline">
                ${globalEvents.length === 0 ? this.renderEmptyState() : this.renderTimeline()}
            </div>

            <div class="legend">
                <div class="legend-item">
                    <div class="legend-color event-root"></div>
                    <span>Root</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-request"></div>
                    <span>Request Step</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-foreach"></div>
                    <span>ForEach Step</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-forvalues"></div>
                    <span>ForValues Step</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-response"></div>
                    <span>Response</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-transform"></div>
                    <span>Transform</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-merge"></div>
                    <span>Merge</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color event-parallel"></div>
                    <span>Parallel</span>
                </div>
            </div>

            <script nonce="${nonce}">
                const vscode = acquireVsCodeApi();
                let zoomLevel = 1;

                ${this.renderTimelineScript()}

                function zoomIn() {
                    zoomLevel = Math.min(zoomLevel * 1.5, 10);
                    applyZoom();
                }

                function zoomOut() {
                    zoomLevel = Math.max(zoomLevel / 1.5, 0.1);
                    applyZoom();
                }

                function resetZoom() {
                    zoomLevel = 1;
                    applyZoom();
                }

                function applyZoom() {
                    const events = document.querySelectorAll('.event-bar, .event-instant');
                    events.forEach(el => {
                        const left = parseFloat(el.dataset.left || '0');
                        const width = parseFloat(el.dataset.width || '0');
                        el.style.left = (left * zoomLevel) + 'px';
                        if (width > 0) {
                            el.style.width = (width * zoomLevel) + 'px';
                        }
                    });

                    const markers = document.querySelectorAll('.time-marker');
                    markers.forEach(el => {
                        const left = parseFloat(el.dataset.left || '0');
                        el.style.left = (left * zoomLevel) + 'px';
                    });
                }

                function selectEvent(eventId) {
                    vscode.postMessage({
                        command: 'selectEvent',
                        eventId: eventId
                    });
                }
            </script>
        </body>
        </html>`;
    }

    private renderEmptyState(): string {
        return `
            <div class="empty-state">
                <p>No execution events yet.</p>
                <p>Run a crawler to see the execution timeline.</p>
            </div>
        `;
    }

    private renderTimeline(): string {
        if (globalEvents.length === 0) {
            return '';
        }

        // Build step groups by pairing START and END events
        const stepGroups = this.buildStepGroups();

        if (stepGroups.length === 0) {
            return '<div class="empty-state"><p>No step durations available yet.</p></div>';
        }

        // Find time bounds from step groups
        const startTime = Math.min(...stepGroups.map(s => s.startTime));
        const endTime = Math.max(...stepGroups.map(s => s.endTime));
        const totalDuration = endTime - startTime;
        const pixelsPerMs = totalDuration > 0 ? 1000 / totalDuration : 1;

        // Render time axis
        const numMarkers = 10;
        let timeAxisHtml = '<div class="time-axis">';
        for (let i = 0; i <= numMarkers; i++) {
            const ms = (totalDuration * i) / numMarkers;
            const time = new Date(startTime + ms);
            const left = ms * pixelsPerMs;
            timeAxisHtml += `<div class="time-marker" data-left="${left}" style="left: ${left}px">${time.toLocaleTimeString()}.${(ms % 1000).toFixed(0).padStart(3, '0')}</div>`;
        }
        timeAxisHtml += '</div>';

        // Render step groups
        const rowHeight = 30;
        const totalHeight = stepGroups.length * rowHeight + 50; // Add padding
        let eventsHtml = `<div class="timeline-events" style="min-height: ${totalHeight}px;">`;
        let rowIndex = 0;

        for (const group of stepGroups) {
            const eventStart = group.startTime - startTime;
            const eventDuration = group.duration;
            const left = eventStart * pixelsPerMs;
            const width = eventDuration * pixelsPerMs;
            const top = rowIndex * rowHeight;
            const eventClass = this.getEventClass(group.type);
            const label = group.label;

            eventsHtml += `
                <div class="event-bar ${eventClass}"
                     data-left="${left}"
                     data-width="${width}"
                     style="left: ${left}px; top: ${top}px; width: ${Math.max(width, 2)}px"
                     onclick="selectEvent('${group.id}')"
                     title="${label} (${eventDuration.toFixed(2)}ms)">
                    ${escapeHtml(label)} ${eventDuration.toFixed(0)}ms
                    ${group.workerId !== undefined ? ` [W${group.workerId}]` : ''}
                </div>
            `;

            rowIndex++;
        }

        eventsHtml += '</div>';

        return timeAxisHtml + eventsHtml;
    }

    private buildStepGroups(): Array<{
        id: string;
        label: string;
        type: number;
        startTime: number;
        endTime: number;
        duration: number;
        workerId?: number;
    }> {
        const groups: Array<{
            id: string;
            label: string;
            type: number;
            startTime: number;
            endTime: number;
            duration: number;
            workerId?: number;
        }> = [];

        // Map to track START events waiting for END events
        const startEvents: Map<string, StepProfilerData> = new Map();

        for (const event of globalEvents) {
            const eventType = event.type;

            // Check if this is a START event
            if (eventType === ProfileEventType.EVENT_REQUEST_STEP_START ||
                eventType === ProfileEventType.EVENT_FOREACH_STEP_START ||
                eventType === ProfileEventType.EVENT_FORVALUES_STEP_START ||
                eventType === ProfileEventType.EVENT_REQUEST_PAGE_START) {
                startEvents.set(event.id, event);
            }
            // Check if this is an END event
            else if (eventType === ProfileEventType.EVENT_REQUEST_STEP_END ||
                     eventType === ProfileEventType.EVENT_FOREACH_STEP_END ||
                     eventType === ProfileEventType.EVENT_FORVALUES_STEP_END ||
                     eventType === ProfileEventType.EVENT_REQUEST_PAGE_END) {

                // Find corresponding START event
                const startEvent = startEvents.get(event.id);
                if (startEvent) {
                    const startTime = new Date(startEvent.timestamp).getTime();
                    const endTime = new Date(event.timestamp).getTime();
                    const duration = endTime - startTime;

                    let label = startEvent.name || 'Step';
                    if (startEvent.data?.stepName) {
                        label = startEvent.data.stepName;
                    } else if (startEvent.data?.pageNumber) {
                        label = `Page ${startEvent.data.pageNumber}`;
                    }

                    groups.push({
                        id: event.id,
                        label: label,
                        type: startEvent.type,
                        startTime: startTime,
                        endTime: endTime,
                        duration: duration,
                        workerId: startEvent.workerId
                    });

                    startEvents.delete(event.id);
                }
            }
        }

        // Sort by start time
        groups.sort((a, b) => a.startTime - b.startTime);

        return groups;
    }

    private renderTimelineScript(): string {
        return `
            // Timeline initialization
            console.log('Timeline loaded with ${globalEvents.length} events');
        `;
    }

    private getEventClass(eventType: number): string {
        switch (eventType) {
            case ProfileEventType.EVENT_ROOT_START:
                return 'event-root';
            case ProfileEventType.EVENT_REQUEST_STEP_START:
            case ProfileEventType.EVENT_REQUEST_STEP_END:
                return 'event-request';
            case ProfileEventType.EVENT_FOREACH_STEP_START:
            case ProfileEventType.EVENT_FOREACH_STEP_END:
                return 'event-foreach';
            case ProfileEventType.EVENT_FORVALUES_STEP_START:
            case ProfileEventType.EVENT_FORVALUES_STEP_END:
                return 'event-forvalues';
            case ProfileEventType.EVENT_REQUEST_PAGE_START:
            case ProfileEventType.EVENT_REQUEST_PAGE_END:
                return 'event-page';
            case ProfileEventType.EVENT_CONTEXT_SELECTION:
                return 'event-context';
            case ProfileEventType.EVENT_URL_COMPOSITION:
                return 'event-url';
            case ProfileEventType.EVENT_REQUEST_DETAILS:
                return 'event-http';
            case ProfileEventType.EVENT_REQUEST_RESPONSE:
                return 'event-response';
            case ProfileEventType.EVENT_RESPONSE_TRANSFORM:
                return 'event-transform';
            case ProfileEventType.EVENT_CONTEXT_MERGE:
                return 'event-merge';
            case ProfileEventType.EVENT_PARALLELISM_SETUP:
                return 'event-parallel';
            case ProfileEventType.EVENT_ITEM_SELECTION:
                return 'event-item';
            case ProfileEventType.EVENT_PAGINATION_EVAL:
                return 'event-pagination';
            case ProfileEventType.EVENT_RESULT:
            case ProfileEventType.EVENT_STREAM_RESULT:
                return 'event-result';
            default:
                return 'event-request';
        }
    }
}
