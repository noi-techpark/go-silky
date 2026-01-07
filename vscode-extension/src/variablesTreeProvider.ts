// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';

export class VariableTreeItem extends vscode.TreeItem {
    constructor(
        public readonly key: string,
        public readonly value: any,
        public readonly collapsibleState: vscode.TreeItemCollapsibleState,
        public readonly isRoot: boolean = false,
        public readonly parentKey?: string
    ) {
        super(key, collapsibleState);

        if (isRoot) {
            // Root-level variable
            this.contextValue = 'variable';
            this.iconPath = new vscode.ThemeIcon('symbol-variable');

            if (typeof value === 'object' && value !== null) {
                this.description = Array.isArray(value)
                    ? `Array[${value.length}]`
                    : `Object{${Object.keys(value).length}}`;
            } else {
                this.description = this.formatValue(value);
            }
            this.tooltip = `${key}: ${JSON.stringify(value, null, 2)}`;
        } else {
            // Nested property
            this.contextValue = 'variableProperty';
            this.iconPath = this.getPropertyIcon(value);
            this.description = this.formatValue(value);
            this.tooltip = `${key}: ${JSON.stringify(value, null, 2)}`;
        }
    }

    private formatValue(value: any): string {
        if (value === null) return 'null';
        if (value === undefined) return 'undefined';
        if (typeof value === 'string') return `"${value}"`;
        if (typeof value === 'object') {
            if (Array.isArray(value)) return `Array[${value.length}]`;
            return `Object{${Object.keys(value).length}}`;
        }
        return String(value);
    }

    private getPropertyIcon(value: any): vscode.ThemeIcon {
        if (value === null || value === undefined) return new vscode.ThemeIcon('circle-slash');
        if (typeof value === 'string') return new vscode.ThemeIcon('symbol-string');
        if (typeof value === 'number') return new vscode.ThemeIcon('symbol-number');
        if (typeof value === 'boolean') return new vscode.ThemeIcon('symbol-boolean');
        if (Array.isArray(value)) return new vscode.ThemeIcon('symbol-array');
        if (typeof value === 'object') return new vscode.ThemeIcon('symbol-object');
        return new vscode.ThemeIcon('symbol-variable');
    }
}

export class VariablesTreeProvider implements vscode.TreeDataProvider<VariableTreeItem> {
    private _onDidChangeTreeData: vscode.EventEmitter<VariableTreeItem | undefined | null | void> = new vscode.EventEmitter<VariableTreeItem | undefined | null | void>();
    readonly onDidChangeTreeData: vscode.Event<VariableTreeItem | undefined | null | void> = this._onDidChangeTreeData.event;

    private variables: Record<string, any> = {};

    constructor() {}

    refresh(): void {
        this._onDidChangeTreeData.fire();
    }

    setVariables(vars: Record<string, any> | undefined): void {
        this.variables = vars || {};
        this.refresh();
    }

    getVariables(): Record<string, any> | undefined {
        if (Object.keys(this.variables).length === 0) {
            return undefined;
        }
        return this.variables;
    }

    addVariable(key: string, value: any): void {
        this.variables[key] = value;
        this.refresh();
    }

    removeVariable(key: string): void {
        delete this.variables[key];
        this.refresh();
    }

    clearVariables(): void {
        this.variables = {};
        this.refresh();
    }

    hasVariables(): boolean {
        return Object.keys(this.variables).length > 0;
    }

    getTreeItem(element: VariableTreeItem): vscode.TreeItem {
        return element;
    }

    getChildren(element?: VariableTreeItem): Thenable<VariableTreeItem[]> {
        if (!element) {
            // Root level - show all variables
            const items: VariableTreeItem[] = [];
            for (const [key, value] of Object.entries(this.variables)) {
                const hasChildren = typeof value === 'object' && value !== null && Object.keys(value).length > 0;
                items.push(new VariableTreeItem(
                    key,
                    value,
                    hasChildren ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None,
                    true
                ));
            }
            return Promise.resolve(items);
        } else {
            // Nested level - show properties of object/array
            const items: VariableTreeItem[] = [];
            const value = element.value;

            if (typeof value === 'object' && value !== null) {
                for (const [key, childValue] of Object.entries(value)) {
                    const hasChildren = typeof childValue === 'object' && childValue !== null && Object.keys(childValue).length > 0;
                    items.push(new VariableTreeItem(
                        Array.isArray(value) ? `[${key}]` : key,
                        childValue,
                        hasChildren ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None,
                        false,
                        element.key
                    ));
                }
            }
            return Promise.resolve(items);
        }
    }

    getParent(element: VariableTreeItem): vscode.ProviderResult<VariableTreeItem> {
        // For simplicity, we don't track parent references
        return null;
    }
}
