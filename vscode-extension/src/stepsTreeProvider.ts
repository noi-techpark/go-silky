// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';
import * as fs from 'fs';

// Match Go's new StepProfilerData struct with flexible Data field
export interface StepProfilerData {
    id: string;                    // UUID4
    parentId?: string;             // Parent step ID for hierarchy
    type: number;                  // ProfileEventType enum value
    name: string;
    step: any;                     // Step configuration
    timestamp: string;
    duration?: number;             // Only in END events (milliseconds)
    workerId?: number;             // For parallel execution
    workerPool?: string;           // Worker pool ID
    data: Record<string, any>;     // Flexible event-specific data
}

// Constants matching Go's new ProfileEventType enum
export const ProfileEventType = {
    // Root
    EVENT_ROOT_START: 0,

    // Request step container
    EVENT_REQUEST_STEP_START: 1,
    EVENT_REQUEST_STEP_END: 2,

    // Request step sub-events
    EVENT_CONTEXT_SELECTION: 3,
    EVENT_REQUEST_PAGE_START: 4,
    EVENT_REQUEST_PAGE_END: 5,
    EVENT_PAGINATION_EVAL: 6,
    EVENT_URL_COMPOSITION: 7,
    EVENT_REQUEST_DETAILS: 8,
    EVENT_REQUEST_RESPONSE: 9,
    EVENT_RESPONSE_TRANSFORM: 10,
    EVENT_CONTEXT_MERGE: 11,

    // ForEach step container
    EVENT_FOREACH_STEP_START: 12,
    EVENT_FOREACH_STEP_END: 13,

    // ForValues step container
    EVENT_FORVALUES_STEP_START: 14,
    EVENT_FORVALUES_STEP_END: 15,

    // ForEach/ForValues step sub-events
    EVENT_PARALLELISM_SETUP: 16,
    EVENT_ITEM_SELECTION: 17,

    // Authentication events
    EVENT_AUTH_START: 18,
    EVENT_AUTH_CACHED: 19,
    EVENT_AUTH_LOGIN_START: 20,
    EVENT_AUTH_LOGIN_END: 21,
    EVENT_AUTH_TOKEN_EXTRACT: 22,
    EVENT_AUTH_TOKEN_INJECT: 23,
    EVENT_AUTH_END: 24,

    // Result events
    EVENT_RESULT: 25,
    EVENT_STREAM_RESULT: 26,

    // Errors
    EVENT_ERROR: 27,

    EVENT_MAX_NUM: 27
} as const;

export class StepTreeItem extends vscode.TreeItem {
    public children: StepTreeItem[] = [];
    public isExpanded: boolean = false;

    constructor(
        public readonly label: string,
        public readonly collapsibleState: vscode.TreeItemCollapsibleState,
        public readonly data: StepProfilerData
    ) {
        super(label, collapsibleState);

        this.id = data.id; // Use UUID as tree item ID
        this.tooltip = this.getTooltip();
        this.iconPath = this.getIcon();
        this.contextValue = 'step';

        // Set description based on duration if available
        if (data.duration) {
            this.description = `${data.duration}ms`;
        } else if (data.workerId !== undefined) {
            this.description = `Worker ${data.workerId}`;
        }

        // Set command to toggle accordion details on click
        this.command = {
            command: 'silky.toggleStepDetails',
            title: 'Toggle Step Details',
            arguments: [this]
        };
    }

    private getTooltip(): string {
        const parts: string[] = [];

        parts.push(`Event: ${this.getEventTypeName()}`);

        if (this.data.name) {
            parts.push(`Name: ${this.data.name}`);
        }

        if (this.data.timestamp) {
            parts.push(`Time: ${this.data.timestamp}`);
        }

        if (this.data.duration !== undefined) {
            parts.push(`Duration: ${this.data.duration}ms`);
        }

        if (this.data.workerId !== undefined) {
            parts.push(`Worker: ${this.data.workerId}`);
        }

        if (this.data.workerPool) {
            parts.push(`Pool: ${this.data.workerPool}`);
        }

        // Add specific data hints based on event type
        const eventData = this.data.data;
        if (eventData) {
            if (eventData.statusCode) {
                parts.push(`Status: ${eventData.statusCode}`);
            }
            if (eventData.url || eventData.resultUrl) {
                parts.push(`URL: ${eventData.url || eventData.resultUrl}`);
            }
            if (eventData.maxConcurrency) {
                parts.push(`Concurrency: ${eventData.maxConcurrency}`);
            }
        }

        return parts.join('\n');
    }

    private getEventTypeName(): string {
        const eventNames: Record<number, string> = {
            [ProfileEventType.EVENT_ROOT_START]: 'Root Start',
            [ProfileEventType.EVENT_REQUEST_STEP_START]: 'Request Step',
            [ProfileEventType.EVENT_REQUEST_STEP_END]: 'Request Step End',
            [ProfileEventType.EVENT_CONTEXT_SELECTION]: 'Context Selection',
            [ProfileEventType.EVENT_REQUEST_PAGE_START]: 'Request Page',
            [ProfileEventType.EVENT_REQUEST_PAGE_END]: 'Request Page End',
            [ProfileEventType.EVENT_PAGINATION_EVAL]: 'Pagination Evaluation',
            [ProfileEventType.EVENT_URL_COMPOSITION]: 'URL Composition',
            [ProfileEventType.EVENT_REQUEST_DETAILS]: 'Request Details',
            [ProfileEventType.EVENT_REQUEST_RESPONSE]: 'Response',
            [ProfileEventType.EVENT_RESPONSE_TRANSFORM]: 'Transform',
            [ProfileEventType.EVENT_CONTEXT_MERGE]: 'Context Merge',
            [ProfileEventType.EVENT_FOREACH_STEP_START]: 'ForEach Step',
            [ProfileEventType.EVENT_FOREACH_STEP_END]: 'ForEach Step End',
            [ProfileEventType.EVENT_FORVALUES_STEP_START]: 'ForValues Step',
            [ProfileEventType.EVENT_FORVALUES_STEP_END]: 'ForValues Step End',
            [ProfileEventType.EVENT_PARALLELISM_SETUP]: 'Parallelism Setup',
            [ProfileEventType.EVENT_ITEM_SELECTION]: 'Item Selection',
            [ProfileEventType.EVENT_AUTH_START]: 'Auth Start',
            [ProfileEventType.EVENT_AUTH_CACHED]: 'Auth Cached',
            [ProfileEventType.EVENT_AUTH_LOGIN_START]: 'Auth Login',
            [ProfileEventType.EVENT_AUTH_LOGIN_END]: 'Auth Login End',
            [ProfileEventType.EVENT_AUTH_TOKEN_EXTRACT]: 'Token Extract',
            [ProfileEventType.EVENT_AUTH_TOKEN_INJECT]: 'Token Inject',
            [ProfileEventType.EVENT_AUTH_END]: 'Auth End',
            [ProfileEventType.EVENT_RESULT]: 'Final Result',
            [ProfileEventType.EVENT_STREAM_RESULT]: 'Stream Result',
            [ProfileEventType.EVENT_ERROR]: 'Error'
        };

        return eventNames[this.data.type] || `Unknown Event (${this.data.type})`;
    }

    private getIcon(): vscode.ThemeIcon {
        // Map event types to icons
        switch (this.data.type) {
            case ProfileEventType.EVENT_ROOT_START:
                return new vscode.ThemeIcon('home');

            case ProfileEventType.EVENT_REQUEST_STEP_START:
            case ProfileEventType.EVENT_REQUEST_STEP_END:
                return new vscode.ThemeIcon('cloud-download');

            case ProfileEventType.EVENT_FOREACH_STEP_START:
            case ProfileEventType.EVENT_FOREACH_STEP_END:
                return new vscode.ThemeIcon('symbol-array');

            case ProfileEventType.EVENT_FORVALUES_STEP_START:
            case ProfileEventType.EVENT_FORVALUES_STEP_END:
                return new vscode.ThemeIcon('list-ordered');

            case ProfileEventType.EVENT_CONTEXT_SELECTION:
                return new vscode.ThemeIcon('arrow-swap');

            case ProfileEventType.EVENT_REQUEST_PAGE_START:
            case ProfileEventType.EVENT_REQUEST_PAGE_END:
                return new vscode.ThemeIcon('file');

            case ProfileEventType.EVENT_PAGINATION_EVAL:
                return new vscode.ThemeIcon('debug-step-over');

            case ProfileEventType.EVENT_URL_COMPOSITION:
                return new vscode.ThemeIcon('link');

            case ProfileEventType.EVENT_REQUEST_DETAILS:
                return new vscode.ThemeIcon('arrow-up');

            case ProfileEventType.EVENT_REQUEST_RESPONSE:
                return new vscode.ThemeIcon('arrow-down');

            case ProfileEventType.EVENT_RESPONSE_TRANSFORM:
                return new vscode.ThemeIcon('symbol-operator');

            case ProfileEventType.EVENT_CONTEXT_MERGE:
                return new vscode.ThemeIcon('merge');

            case ProfileEventType.EVENT_PARALLELISM_SETUP:
                return new vscode.ThemeIcon('multiple-windows');

            case ProfileEventType.EVENT_ITEM_SELECTION:
                return new vscode.ThemeIcon('symbol-event');

            case ProfileEventType.EVENT_AUTH_START:
            case ProfileEventType.EVENT_AUTH_END:
                return new vscode.ThemeIcon('shield');

            case ProfileEventType.EVENT_AUTH_CACHED:
                return new vscode.ThemeIcon('database');

            case ProfileEventType.EVENT_AUTH_LOGIN_START:
            case ProfileEventType.EVENT_AUTH_LOGIN_END:
                return new vscode.ThemeIcon('sign-in');

            case ProfileEventType.EVENT_AUTH_TOKEN_EXTRACT:
                return new vscode.ThemeIcon('key');

            case ProfileEventType.EVENT_AUTH_TOKEN_INJECT:
                return new vscode.ThemeIcon('arrow-small-right');

            case ProfileEventType.EVENT_RESULT:
            case ProfileEventType.EVENT_STREAM_RESULT:
                return new vscode.ThemeIcon('check');

            case ProfileEventType.EVENT_ERROR:
                return new vscode.ThemeIcon('error');

            default:
                return new vscode.ThemeIcon('circle-outline');
        }
    }

    get status(): string {
        return this.getEventTypeName();
    }
}

export class StepsTreeProvider implements vscode.TreeDataProvider<StepTreeItem> {
    private _onDidChangeTreeData: vscode.EventEmitter<StepTreeItem | undefined | null | void> =
        new vscode.EventEmitter<StepTreeItem | undefined | null | void>();
    readonly onDidChangeTreeData: vscode.Event<StepTreeItem | undefined | null | void> =
        this._onDidChangeTreeData.event;

    private rootSteps: StepTreeItem[] = [];

    // ID-based hierarchy tracking
    private stepMap: Map<string, StepTreeItem> = new Map();

    // Track START events waiting for END event enrichment
    private pendingStartEvents: Map<string, StepTreeItem> = new Map();

    // Timeline callback
    private timelineCallback?: (event: StepProfilerData) => void;

    public setTimelineCallback(callback: (event: StepProfilerData) => void) {
        this.timelineCallback = callback;
    }

    refresh(): void {
        this._onDidChangeTreeData.fire();
    }

    clear(): void {
        this.rootSteps = [];
        this.stepMap.clear();
        this.pendingStartEvents.clear();
        this.refresh();
    }

    collapseAll(): void {
        // VSCode handles this automatically with workbench.actions.treeView.collapseAll
        // Just trigger a refresh to ensure the view is up to date
        this.refresh();
    }

    hasSteps(): boolean {
        return this.rootSteps.length > 0;
    }

    getTreeItem(element: StepTreeItem): vscode.TreeItem {
        return element;
    }

    getChildren(element?: StepTreeItem): Thenable<StepTreeItem[]> {
        if (!element) {
            return Promise.resolve(this.rootSteps);
        }
        return Promise.resolve(element.children);
    }

    getParent(element: StepTreeItem): vscode.ProviderResult<StepTreeItem> {
        // Required for reveal() to work - traverse up the tree hierarchy
        if (element.data.parentId) {
            return this.stepMap.get(element.data.parentId);
        }
        return null;
    }

    addStep(profilerData: StepProfilerData): void {
        const eventType = profilerData.type;

        // Notify timeline of all events (including END events)
        if (this.timelineCallback) {
            this.timelineCallback(profilerData);
        }

        // Handle END events - enrich the corresponding START event with duration
        if (eventType === ProfileEventType.EVENT_REQUEST_STEP_END ||
            eventType === ProfileEventType.EVENT_FOREACH_STEP_END ||
            eventType === ProfileEventType.EVENT_FORVALUES_STEP_END ||
            eventType === ProfileEventType.EVENT_AUTH_LOGIN_END ||
            eventType === ProfileEventType.EVENT_AUTH_END ||
            eventType === ProfileEventType.EVENT_REQUEST_PAGE_END) {

            // Find the corresponding START event by ID
            // END events share the same ID as their START events in our system
            const startItem = this.stepMap.get(profilerData.id);
            if (startItem) {
                // Calculate duration from timestamps if not provided
                let duration = profilerData.duration;
                if (duration === undefined) {
                    const startTime = new Date(startItem.data.timestamp).getTime();
                    const endTime = new Date(profilerData.timestamp).getTime();
                    duration = endTime - startTime;
                }

                // Enrich the START event with duration
                startItem.data.duration = duration;
                startItem.description = `${duration}ms`;

                // Update tooltip to include duration
                startItem.tooltip = this.getUpdatedTooltip(startItem);

                // Fire change event for the entire tree to ensure update is visible
                this._onDidChangeTreeData.fire(undefined);
            }
            return; // Don't create a separate tree item for END events
        }

        // Determine label based on event type
        let label = profilerData.name || this.getDefaultLabel(eventType, profilerData);

        // Determine collapsible state - container events that can have children
        let collapsibleState = vscode.TreeItemCollapsibleState.None;
        if (eventType === ProfileEventType.EVENT_ROOT_START ||
            eventType === ProfileEventType.EVENT_REQUEST_STEP_START ||
            eventType === ProfileEventType.EVENT_FOREACH_STEP_START ||
            eventType === ProfileEventType.EVENT_FORVALUES_STEP_START ||
            eventType === ProfileEventType.EVENT_REQUEST_PAGE_START ||
            eventType === ProfileEventType.EVENT_AUTH_START ||
            eventType === ProfileEventType.EVENT_AUTH_LOGIN_START ||
            eventType === ProfileEventType.EVENT_ITEM_SELECTION) {
            collapsibleState = vscode.TreeItemCollapsibleState.Expanded;
        }

        // Create new tree item
        const newItem = new StepTreeItem(label, collapsibleState, profilerData);

        // Store in map for quick lookup
        this.stepMap.set(profilerData.id, newItem);

        // Track START events for later enrichment
        if (eventType === ProfileEventType.EVENT_REQUEST_STEP_START ||
            eventType === ProfileEventType.EVENT_FOREACH_STEP_START ||
            eventType === ProfileEventType.EVENT_FORVALUES_STEP_START ||
            eventType === ProfileEventType.EVENT_AUTH_START ||
            eventType === ProfileEventType.EVENT_AUTH_LOGIN_START ||
            eventType === ProfileEventType.EVENT_REQUEST_PAGE_START) {
            this.pendingStartEvents.set(profilerData.id, newItem);
        }

        // Add to hierarchy based on parentId
        if (!profilerData.parentId) {
            // Root-level item
            this.rootSteps.push(newItem);
        } else {
            // Find parent and add as child
            const parent = this.stepMap.get(profilerData.parentId);
            if (parent) {
                parent.children.push(newItem);
            } else {
                // Parent not found yet (shouldn't happen with proper ordering)
                // Add to root as fallback
                this.rootSteps.push(newItem);
            }
        }

        // Refresh the tree view to show the new item
        this.refresh();
    }

    private getDefaultLabel(eventType: number, data: StepProfilerData): string {
        // Generate default labels based on event type and data
        switch (eventType) {
            case ProfileEventType.EVENT_ROOT_START:
                return 'Root';
            case ProfileEventType.EVENT_REQUEST_STEP_START:
                return data.data?.stepName || 'Request Step';
            case ProfileEventType.EVENT_FOREACH_STEP_START:
                return data.data?.stepName || 'ForEach Step';
            case ProfileEventType.EVENT_FORVALUES_STEP_START:
                return data.data?.stepName || 'ForValues Step';
            case ProfileEventType.EVENT_CONTEXT_SELECTION:
                return `Context: ${data.data?.currentContextKey || 'unknown'}`;
            case ProfileEventType.EVENT_REQUEST_PAGE_START:
                return `Page ${data.data?.pageNumber || '?'}`;
            case ProfileEventType.EVENT_PAGINATION_EVAL:
                return `Pagination (Page ${data.data?.pageNumber || '?'})`;
            case ProfileEventType.EVENT_URL_COMPOSITION:
                return 'URL Composition';
            case ProfileEventType.EVENT_REQUEST_DETAILS:
                return `${data.data?.method || 'REQUEST'}`;
            case ProfileEventType.EVENT_REQUEST_RESPONSE:
                return `Response ${data.data?.statusCode || ''}`;
            case ProfileEventType.EVENT_RESPONSE_TRANSFORM:
                return 'Transform';
            case ProfileEventType.EVENT_CONTEXT_MERGE:
                return 'Merge';
            case ProfileEventType.EVENT_PARALLELISM_SETUP:
                return `Parallel (${data.data?.maxConcurrency || '?'} workers)`;
            case ProfileEventType.EVENT_ITEM_SELECTION:
                return `Item ${data.data?.iterationIndex ?? '?'}`;
            case ProfileEventType.EVENT_AUTH_START:
                return `Auth: ${data.data?.authType || 'unknown'}`;
            case ProfileEventType.EVENT_AUTH_CACHED:
                return `Cached Token`;
            case ProfileEventType.EVENT_AUTH_LOGIN_START:
                return `Login Request`;
            case ProfileEventType.EVENT_AUTH_LOGIN_END:
                return `Login ${data.data?.error ? 'Failed' : 'Complete'}`;
            case ProfileEventType.EVENT_AUTH_TOKEN_EXTRACT:
                return `Extract Token`;
            case ProfileEventType.EVENT_AUTH_TOKEN_INJECT:
                return `Inject Token`;
            case ProfileEventType.EVENT_AUTH_END:
                return `Auth Complete`;
            case ProfileEventType.EVENT_RESULT:
                return 'Final Result';
            case ProfileEventType.EVENT_STREAM_RESULT:
                return `Stream Result ${data.data?.index ?? ''}`;
            default:
                return 'Step';
        }
    }

    async exportSteps(filePath: string): Promise<void> {
        const exportData = this.serializeSteps(this.rootSteps);
        await fs.promises.writeFile(filePath, JSON.stringify(exportData, null, 2));
    }

    private serializeSteps(steps: StepTreeItem[]): any[] {
        return steps.map(step => ({
            id: step.data.id,
            parentId: step.data.parentId,
            type: step.data.type,
            label: step.label,
            name: step.data.name,
            timestamp: step.data.timestamp,
            duration: step.data.duration,
            workerId: step.data.workerId,
            workerPool: step.data.workerPool,
            data: step.data.data,
            step: step.data.step,
            children: this.serializeSteps(step.children)
        }));
    }

    getSteps(): StepTreeItem[] {
        return this.rootSteps;
    }

    getStepById(id: string): StepTreeItem | undefined {
        return this.stepMap.get(id);
    }

    getAllSteps(): StepTreeItem[] {
        return Array.from(this.stepMap.values());
    }

    private getUpdatedTooltip(item: StepTreeItem): string {
        const parts: string[] = [];
        const data = item.data;

        // Event type name
        const eventNames: Record<number, string> = {
            [ProfileEventType.EVENT_ROOT_START]: 'Root Start',
            [ProfileEventType.EVENT_REQUEST_STEP_START]: 'Request Step',
            [ProfileEventType.EVENT_REQUEST_STEP_END]: 'Request Step End',
            [ProfileEventType.EVENT_CONTEXT_SELECTION]: 'Context Selection',
            [ProfileEventType.EVENT_REQUEST_PAGE_START]: 'Request Page',
            [ProfileEventType.EVENT_REQUEST_PAGE_END]: 'Request Page End',
            [ProfileEventType.EVENT_PAGINATION_EVAL]: 'Pagination Evaluation',
            [ProfileEventType.EVENT_URL_COMPOSITION]: 'URL Composition',
            [ProfileEventType.EVENT_REQUEST_DETAILS]: 'Request Details',
            [ProfileEventType.EVENT_REQUEST_RESPONSE]: 'Response',
            [ProfileEventType.EVENT_RESPONSE_TRANSFORM]: 'Transform',
            [ProfileEventType.EVENT_CONTEXT_MERGE]: 'Context Merge',
            [ProfileEventType.EVENT_FOREACH_STEP_START]: 'ForEach Step',
            [ProfileEventType.EVENT_FOREACH_STEP_END]: 'ForEach Step End',
            [ProfileEventType.EVENT_FORVALUES_STEP_START]: 'ForValues Step',
            [ProfileEventType.EVENT_FORVALUES_STEP_END]: 'ForValues Step End',
            [ProfileEventType.EVENT_PARALLELISM_SETUP]: 'Parallelism Setup',
            [ProfileEventType.EVENT_ITEM_SELECTION]: 'Item Selection',
            [ProfileEventType.EVENT_AUTH_START]: 'Auth Start',
            [ProfileEventType.EVENT_AUTH_CACHED]: 'Auth Cached',
            [ProfileEventType.EVENT_AUTH_LOGIN_START]: 'Auth Login',
            [ProfileEventType.EVENT_AUTH_LOGIN_END]: 'Auth Login End',
            [ProfileEventType.EVENT_AUTH_TOKEN_EXTRACT]: 'Token Extract',
            [ProfileEventType.EVENT_AUTH_TOKEN_INJECT]: 'Token Inject',
            [ProfileEventType.EVENT_AUTH_END]: 'Auth End',
            [ProfileEventType.EVENT_RESULT]: 'Final Result',
            [ProfileEventType.EVENT_STREAM_RESULT]: 'Stream Result',
            [ProfileEventType.EVENT_ERROR]: 'Error'
        };

        parts.push(`Event: ${eventNames[data.type] || `Unknown Event (${data.type})`}`);

        if (data.name) {
            parts.push(`Name: ${data.name}`);
        }

        if (data.timestamp) {
            parts.push(`Time: ${data.timestamp}`);
        }

        if (data.duration !== undefined) {
            parts.push(`Duration: ${data.duration}ms`);
        }

        if (data.workerId !== undefined) {
            parts.push(`Worker: ${data.workerId}`);
        }

        if (data.workerPool) {
            parts.push(`Pool: ${data.workerPool}`);
        }

        // Add specific data hints based on event type
        const eventData = data.data;
        if (eventData) {
            if (eventData.statusCode) {
                parts.push(`Status: ${eventData.statusCode}`);
            }
            if (eventData.url || eventData.resultUrl) {
                parts.push(`URL: ${eventData.url || eventData.resultUrl}`);
            }
            if (eventData.maxConcurrency) {
                parts.push(`Concurrency: ${eventData.maxConcurrency}`);
            }
        }

        return parts.join('\n');
    }
}
