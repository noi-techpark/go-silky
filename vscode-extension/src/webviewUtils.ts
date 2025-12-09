// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';

/**
 * Gets the URI for a webview resource in the media folder
 */
export function getMediaUri(webview: vscode.Webview, extensionUri: vscode.Uri, ...pathSegments: string[]): vscode.Uri {
    return webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'media', ...pathSegments));
}

/**
 * Gets a nonce for CSP-compliant inline scripts
 */
export function getNonce(): string {
    let text = '';
    const possible = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    for (let i = 0; i < 32; i++) {
        text += possible.charAt(Math.floor(Math.random() * possible.length));
    }
    return text;
}

/**
 * Escapes HTML special characters to prevent XSS
 */
export function escapeHtml(text: string): string {
    return text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;')
        .replace(/`/g, '&#096;');
}

/**
 * Gets HTTP status text for a status code
 */
export function getStatusText(code: number): string {
    const codes: Record<number, string> = {
        200: 'OK',
        201: 'Created',
        204: 'No Content',
        400: 'Bad Request',
        401: 'Unauthorized',
        403: 'Forbidden',
        404: 'Not Found',
        500: 'Internal Server Error',
        502: 'Bad Gateway',
        503: 'Service Unavailable'
    };
    return codes[code] || '';
}

/**
 * Formats bytes to a human-readable string
 */
export function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
}

/**
 * Reads a CSS file from the media/styles directory
 */
export function readCssFile(extensionPath: string, filename: string): string {
    const cssPath = path.join(extensionPath, 'media', 'styles', filename);
    try {
        return fs.readFileSync(cssPath, 'utf8');
    } catch (error) {
        console.error(`Failed to read CSS file: ${cssPath}`, error);
        return '';
    }
}

/**
 * Generates a link tag for an external CSS file in webview
 */
export function getCssLink(webview: vscode.Webview, extensionUri: vscode.Uri, ...pathSegments: string[]): string {
    const styleUri = getMediaUri(webview, extensionUri, 'styles', ...pathSegments);
    return `<link rel="stylesheet" type="text/css" href="${styleUri}">`;
}

/**
 * Generates the common HTML head with CSP and external CSS
 */
export function getWebviewHead(webview: vscode.Webview, extensionUri: vscode.Uri, nonce: string, cssFiles: string[] = []): string {
    const cssLinks = cssFiles.map(file => getCssLink(webview, extensionUri, file)).join('\n');

    return `
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
        ${cssLinks}
    `;
}
