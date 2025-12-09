import * as vscode from 'vscode';
import { StepProfilerData, ProfileEventType, StepsTreeProvider } from './stepsTreeProvider';
import { getNonce, escapeHtml, getMediaUri } from './webviewUtils';

// Store events globally so they persist across view lifecycles
let globalEvents: StepProfilerData[] = [];

// Track collapsed state for each event group by ID
let collapsedGroups: Set<string> = new Set();

export class TimelineViewProvider implements vscode.WebviewViewProvider {
    public static readonly viewType = 'silky.timeline';
    private _view?: vscode.WebviewView;
    private _outputChannel: vscode.OutputChannel;

    constructor(
        private readonly _extensionUri: vscode.Uri,
        private readonly _stepsTreeProvider: StepsTreeProvider,
        outputChannel: vscode.OutputChannel
    ) {
        this._outputChannel = outputChannel;
        this._outputChannel.appendLine('------------------------');
    }

    public resolveWebviewView(
        webviewView: vscode.WebviewView,
        _context: vscode.WebviewViewResolveContext,
        _token: vscode.CancellationToken,
    ) {
        this._view = webviewView;

        webviewView.webview.options = {
            enableScripts: true,
            localResourceRoots: [this._extensionUri]
        };

        webviewView.webview.html = this._getHtmlForWebview(webviewView.webview);

        webviewView.webview.onDidReceiveMessage(message => {
            this._outputChannel.appendLine('[TimelineViewProvider] Received message: ' + JSON.stringify(message));
            switch (message.command) {
                case 'selectEvent':
                    this._outputChannel.appendLine('[TimelineViewProvider] Executing selectStepFromTimeline with eventId: ' + message.eventId);
                    // Execute command to select step in tree and show details
                    vscode.commands.executeCommand('silky.selectStepFromTimeline', message.eventId);
                    break;
                case 'toggleCollapse':
                    this._outputChannel.appendLine('[TimelineViewProvider] Toggling collapse for eventId: ' + message.eventId);
                    // Toggle collapse state
                    if (collapsedGroups.has(message.eventId)) {
                        collapsedGroups.delete(message.eventId);
                    } else {
                        collapsedGroups.add(message.eventId);
                    }
                    // Refresh the timeline
                    this.refresh();
                    break;
            }
        });
    }

    public addEvent(event: StepProfilerData) {
        globalEvents.push(event);

        // Log ALL events to understand what we're receiving
        this._outputChannel.appendLine('[Timeline addEvent] Event: ' + event.name + ' type: ' + event.type + ' duration: ' + (event.duration || 'undefined'));

        // Log when we receive events with durations (END events)
        if (event.duration !== undefined) {
            this._outputChannel.appendLine('[Timeline addEvent] *** END EVENT with duration: ' + event.duration + 'ms');

            // Check if the tree has been updated with this duration
            const treeStep = this._stepsTreeProvider.getStepById(event.id);
            if (treeStep) {
                this._outputChannel.appendLine('[Timeline addEvent] Tree step duration: ' + treeStep.data.duration);
            } else {
                this._outputChannel.appendLine('[Timeline addEvent] Step not found in tree!');
            }
        }

        this.refresh();
    }

    public clear() {
        globalEvents = [];
        collapsedGroups.clear();
        this.refresh();
    }

    private refresh() {
        if (this._view) {
            this._view.webview.html = this._getHtmlForWebview(this._view.webview);
        }
    }

    private _getHtmlForWebview(webview: vscode.Webview) {
        const nonce = getNonce();
        const styleUri = getMediaUri(webview, this._extensionUri, 'styles', 'timeline.css');

        return `<!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
            <title>Execution Timeline</title>
            <link rel="stylesheet" type="text/css" href="${styleUri}">
        </head>
        <body>
            <div class="header">
                <h4 style="margin: 0;">Timeline</h4>
                <div class="controls">
                    <button class="btn btn-zoom-in">🔍+</button>
                    <button class="btn btn-zoom-out">🔍-</button>
                    <button class="btn btn-zoom-reset">↺</button>
                </div>
            </div>

            <div id="timeline">
                ${globalEvents.length === 0 ? this.renderEmptyState() : this.renderTimeline()}
            </div>

            <script nonce="${nonce}">
                const vscode = acquireVsCodeApi();
                let zoomLevel = 1;

                ${this.renderTimelineScript()}

                function applyZoom() {
                    const events = document.querySelectorAll('.event-bar');
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

                // Setup event listeners when DOM is ready
                document.addEventListener('DOMContentLoaded', function() {
                    // Zoom in button
                    const zoomInBtn = document.querySelector('.btn-zoom-in');
                    if (zoomInBtn) {
                        zoomInBtn.addEventListener('click', function() {
                            zoomLevel = Math.min(zoomLevel * 1.5, 10);
                            applyZoom();
                        });
                    }

                    // Zoom out button
                    const zoomOutBtn = document.querySelector('.btn-zoom-out');
                    if (zoomOutBtn) {
                        zoomOutBtn.addEventListener('click', function() {
                            zoomLevel = Math.max(zoomLevel / 1.5, 0.1);
                            applyZoom();
                        });
                    }

                    // Reset zoom button
                    const resetBtn = document.querySelector('.btn-zoom-reset');
                    if (resetBtn) {
                        resetBtn.addEventListener('click', function() {
                            zoomLevel = 1;
                            applyZoom();
                        });
                    }

                    // Collapse icon click handlers (with event stop propagation)
                    document.querySelectorAll('.collapse-icon').forEach(icon => {
                        icon.addEventListener('click', function(e) {
                            e.stopPropagation(); // Prevent event bar click
                            const collapseId = this.getAttribute('data-collapse-id');
                            console.log('[Timeline] Toggling collapse for:', collapseId);
                            if (collapseId) {
                                vscode.postMessage({
                                    command: 'toggleCollapse',
                                    eventId: collapseId
                                });
                            }
                        });
                    });

                    // Event bars click handlers
                    document.querySelectorAll('.event-bar').forEach(bar => {
                        bar.addEventListener('click', function() {
                            const eventId = this.getAttribute('data-event-id');
                            console.log('[Timeline] Clicked bar with eventId:', eventId);
                            if (eventId) {
                                console.log('[Timeline] Sending selectEvent message with eventId:', eventId);
                                vscode.postMessage({
                                    command: 'selectEvent',
                                    eventId: eventId
                                });
                            } else {
                                console.error('[Timeline] No eventId found on clicked bar');
                            }
                        });
                    });
                });
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

        this._outputChannel.appendLine('[Timeline renderTimeline] Built ' + stepGroups.length + ' groups');
        if (stepGroups.length > 0) {
            this._outputChannel.appendLine('[Timeline renderTimeline] First group: ' + stepGroups[0].label + ' duration: ' + stepGroups[0].duration);
        }

        if (stepGroups.length === 0) {
            return '<div class="empty-state"><p>No step durations available yet.</p></div>';
        }

        // Find time bounds from step groups
        const startTime = Math.min(...stepGroups.map(s => s.startTime));
        const endTime = Math.max(...stepGroups.map(s => s.endTime));
        const totalDuration = endTime - startTime;
        const pixelsPerMs = totalDuration > 0 ? 800 / totalDuration : 1;

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

        // Helper to count ALL descendants recursively
        const countAllDescendants = (id: string): number => {
            const g = stepGroups.find(g => g.id === id);
            if (!g || g.children.length === 0) {
                return 0;
            }
            let count = g.children.length;
            for (const childId of g.children) {
                count += countAllDescendants(childId);
            }
            return count;
        };

        // Filter visible groups based on collapse state
        const collapsedSet = new Set<string>();
        const collapsedCounts = new Map<string, number>();

        for (const group of stepGroups) {
            if (collapsedGroups.has(group.id)) {
                this._outputChannel.appendLine('[Timeline] Collapsing: ' + group.label + ' (id: ' + group.id.substring(0, 8) + ')');

                // Count all descendants before hiding them
                const totalDescendants = countAllDescendants(group.id);
                collapsedCounts.set(group.id, totalDescendants);
                this._outputChannel.appendLine('[Timeline]   Total descendants to hide: ' + totalDescendants);

                // Add all descendants to collapsed set (recursively)
                const addDescendants = (id: string, indent: number = 2) => {
                    const g = stepGroups.find(g => g.id === id);
                    if (g) {
                        this._outputChannel.appendLine('[Timeline] ' + '  '.repeat(indent) + 'Hiding children of ' + g.label + ': ' + g.children.length + ' direct children');
                        for (const childId of g.children) {
                            collapsedSet.add(childId);
                            const child = stepGroups.find(cg => cg.id === childId);
                            if (child) {
                                this._outputChannel.appendLine('[Timeline] ' + '  '.repeat(indent + 1) + '- ' + child.label);
                            }
                            addDescendants(childId, indent + 1);
                        }
                    }
                };
                addDescendants(group.id);
            }
        }

        const visibleGroups = stepGroups.filter(g => !collapsedSet.has(g.id));

        // Render step groups
        const rowHeight = 26;
        const totalHeight = visibleGroups.length * rowHeight + 40;
        let eventsHtml = `<div class="timeline-events" style="min-height: ${totalHeight}px;">`;
        let rowIndex = 0;

        for (const group of visibleGroups) {
            const eventStart = group.startTime - startTime;
            const eventDuration = group.duration;
            const left = eventStart * pixelsPerMs;
            const width = eventDuration * pixelsPerMs;
            const top = rowIndex * rowHeight;
            const eventClass = this.getEventClass(group.type);
            const label = group.label;

            const hasChildren = group.children.length > 0;
            const isCollapsed = collapsedGroups.has(group.id);
            const collapseIcon = hasChildren ? (isCollapsed ? '▶' : '▼') : '';
            const totalHidden = collapsedCounts.get(group.id) || 0;
            const collapsedCount = isCollapsed ? ` (${totalHidden} hidden)` : '';

            eventsHtml += `
                <div class="event-bar ${eventClass}"
                     data-left="${left}"
                     data-width="${width}"
                     data-event-id="${group.id}"
                     style="left: ${left}px; top: ${top}px; width: ${Math.max(width, 2)}px"
                     title="${label} (${eventDuration.toFixed(2)}ms)">
                    ${hasChildren ? `<span class="collapse-icon" data-collapse-id="${group.id}">${collapseIcon}</span>` : ''}
                    ${escapeHtml(label)} ${eventDuration.toFixed(0)}ms${collapsedCount}
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
        parentId?: string;
        children: string[];
    }> {
        // Get all steps from the tree provider (which has correct hierarchy)
        const allSteps = this._stepsTreeProvider.getAllSteps();
        this._outputChannel.appendLine('[Timeline] buildStepGroups - Total steps: ' + allSteps.length);

        // Debug: show first few steps
        allSteps.slice(0, 10).forEach(s => {
            this._outputChannel.appendLine('[Timeline]   Step: ' + s.label + ' type: ' + s.data.type + ' duration: ' + s.data.duration + ' hasChildren: ' + (s.children.length > 0));
        });

        const groups: Array<{
            id: string;
            label: string;
            type: number;
            startTime: number;
            endTime: number;
            duration: number;
            workerId?: number;
            parentId?: string;
            children: string[];
        }> = [];

        // Build groups from container steps with durations
        for (const step of allSteps) {
            const data = step.data;

            // Only include container steps (Root, Request, ForEach, ForValues, Page) that appear in timeline
            const isContainerStep = (
                data.type === ProfileEventType.EVENT_ROOT_START ||
                data.type === ProfileEventType.EVENT_REQUEST_STEP_START ||
                data.type === ProfileEventType.EVENT_FOREACH_STEP_START ||
                data.type === ProfileEventType.EVENT_FORVALUES_STEP_START ||
                data.type === ProfileEventType.EVENT_REQUEST_PAGE_START
            );

            if (!isContainerStep) {
                continue; // Skip non-container events (details, responses, etc.)
            }

            this._outputChannel.appendLine('[Timeline] Found container step: ' + step.label + ' type: ' + data.type + ' duration: ' + data.duration + ' children: ' + step.children.length);

            // Must have duration (except root which we calculate)
            if (!data.duration && data.type !== ProfileEventType.EVENT_ROOT_START) {
                this._outputChannel.appendLine('[Timeline] Skipping step without duration: ' + step.label);
                continue;
            }

            const timestamp = new Date(data.timestamp).getTime();
            let duration = data.duration || 0;
            let endTime = timestamp + duration;

            // For root, calculate duration from all descendants recursively
            if (data.type === ProfileEventType.EVENT_ROOT_START && !data.duration) {
                this._outputChannel.appendLine('[Timeline] Calculating root duration from children...');
                const getAllDescendantEndTimes = (treeItem: any): number[] => {
                    const times: number[] = [];
                    if (treeItem.data.duration !== undefined) {
                        const start = new Date(treeItem.data.timestamp).getTime();
                        times.push(start + treeItem.data.duration);
                    }
                    for (const child of treeItem.children) {
                        times.push(...getAllDescendantEndTimes(child));
                    }
                    return times;
                };

                const allEndTimes = getAllDescendantEndTimes(step);
                this._outputChannel.appendLine('[Timeline] Root - found ' + allEndTimes.length + ' descendant end times');
                if (allEndTimes.length > 0) {
                    endTime = Math.max(...allEndTimes);
                    duration = endTime - timestamp;
                    this._outputChannel.appendLine('[Timeline] Root calculated duration: ' + duration + ' ms');
                }
            }

            // Recursively collect all container-type descendants (not just direct children)
            // This handles cases like forEach/forValues where Item Selection events are intermediaries
            const collectContainerDescendants = (treeItem: any): string[] => {
                const containerIds: string[] = [];
                for (const child of treeItem.children) {
                    const isContainer = (
                        child.data.type === ProfileEventType.EVENT_ROOT_START ||
                        child.data.type === ProfileEventType.EVENT_REQUEST_STEP_START ||
                        child.data.type === ProfileEventType.EVENT_FOREACH_STEP_START ||
                        child.data.type === ProfileEventType.EVENT_FORVALUES_STEP_START ||
                        child.data.type === ProfileEventType.EVENT_REQUEST_PAGE_START
                    );

                    if (isContainer) {
                        containerIds.push(child.data.id);
                    } else {
                        // Not a container (e.g., Item Selection), recurse into its children
                        containerIds.push(...collectContainerDescendants(child));
                    }
                }
                return containerIds;
            };

            const containerChildren = collectContainerDescendants(step);

            this._outputChannel.appendLine('[Timeline] Adding group: ' + step.label + ' with ' + containerChildren.length + ' container children');
            if (containerChildren.length > 0) {
                this._outputChannel.appendLine('[Timeline]   Children IDs: ' + containerChildren.slice(0, 3).join(', ') + (containerChildren.length > 3 ? '...' : ''));
            }

            groups.push({
                id: data.id,
                label: step.label as string,
                type: data.type,
                startTime: timestamp,
                endTime: endTime,
                duration: duration,
                workerId: data.workerId,
                parentId: data.parentId,
                children: containerChildren
            });
        }

        // Sort by start time for display order
        groups.sort((a, b) => a.startTime - b.startTime);

        this._outputChannel.appendLine('[Timeline] buildStepGroups - Created ' + groups.length + ' groups');
        groups.forEach(g => this._outputChannel.appendLine('[Timeline]   - ' + g.label + ' ' + g.duration + 'ms children: ' + g.children.length));

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
            default:
                return 'event-request';
        }
    }
}
