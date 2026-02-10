// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * JSON Tree Renderer - Interactive collapsible JSON tree for VS Code webviews.
 *
 * Finds all <div class="json-tree"> elements, reads their companion
 * <script type="application/json"> tag, and renders an interactive tree.
 */
(function () {
    'use strict';

    const DEFAULT_DEPTH = 2;
    const ARRAY_TRUNCATE_LIMIT = 100;

    // --- helpers -----------------------------------------------------------

    function getType(val) {
        if (val === null) return 'null';
        if (Array.isArray(val)) return 'array';
        return typeof val;
    }

    function escapeHtml(str) {
        return str
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    function preview(val) {
        var type = getType(val);
        if (type === 'object') {
            var keys = Object.keys(val);
            return '{...} (' + keys.length + (keys.length === 1 ? ' key' : ' keys') + ')';
        }
        if (type === 'array') {
            return '[...] (' + val.length + (val.length === 1 ? ' item' : ' items') + ')';
        }
        return '';
    }

    // --- node builders -----------------------------------------------------

    function createValueSpan(val) {
        var span = document.createElement('span');
        var type = getType(val);
        span.className = 'jt-value jt-' + type;

        if (type === 'string') {
            span.textContent = '"' + val + '"';
        } else if (type === 'null') {
            span.textContent = 'null';
        } else {
            span.textContent = String(val);
        }
        return span;
    }

    function createCopyBtn(data) {
        var btn = document.createElement('button');
        btn.className = 'jt-copy-btn';
        btn.textContent = 'Copy';
        btn.title = 'Copy this subtree as JSON';
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            var json = JSON.stringify(data, null, 2);
            if (typeof vscode !== 'undefined') {
                vscode.postMessage({ command: 'copy', text: json });
            }
        });
        return btn;
    }

    function buildNode(key, val, depth, maxDepth) {
        var type = getType(val);
        var isComplex = (type === 'object' || type === 'array');

        var li = document.createElement('li');
        li.className = 'jt-node';

        var row = document.createElement('div');
        row.className = 'jt-row';
        li.appendChild(row);

        if (isComplex) {
            var toggle = document.createElement('span');
            toggle.className = 'jt-toggle';
            toggle.textContent = depth < maxDepth ? '\u25BC' : '\u25B6'; // ▼ or ▶
            row.appendChild(toggle);

            if (key !== null) {
                var keySpan = document.createElement('span');
                keySpan.className = 'jt-key';
                keySpan.textContent = '"' + key + '"';
                row.appendChild(keySpan);

                var colon = document.createElement('span');
                colon.className = 'jt-colon';
                colon.textContent = ': ';
                row.appendChild(colon);
            }

            var previewSpan = document.createElement('span');
            previewSpan.className = 'jt-preview';
            previewSpan.textContent = preview(val);
            row.appendChild(previewSpan);

            row.appendChild(createCopyBtn(val));

            // Build children
            var childUl = document.createElement('ul');
            childUl.className = 'jt-children';
            if (depth >= maxDepth) {
                childUl.style.display = 'none';
            }
            li.appendChild(childUl);

            if (type === 'object') {
                var keys = Object.keys(val);
                keys.forEach(function (k) {
                    childUl.appendChild(buildNode(k, val[k], depth + 1, maxDepth));
                });
            } else {
                // Array
                var limit = Math.min(val.length, ARRAY_TRUNCATE_LIMIT);
                for (var i = 0; i < limit; i++) {
                    childUl.appendChild(buildNode(String(i), val[i], depth + 1, maxDepth));
                }
                if (val.length > ARRAY_TRUNCATE_LIMIT) {
                    var moreLi = document.createElement('li');
                    moreLi.className = 'jt-node jt-more';
                    var moreBtn = document.createElement('button');
                    moreBtn.className = 'jt-show-more';
                    moreBtn.textContent = 'Show ' + (val.length - ARRAY_TRUNCATE_LIMIT) + ' more...';
                    moreBtn.addEventListener('click', function (e) {
                        e.stopPropagation();
                        for (var j = ARRAY_TRUNCATE_LIMIT; j < val.length; j++) {
                            childUl.insertBefore(
                                buildNode(String(j), val[j], depth + 1, maxDepth),
                                moreLi
                            );
                        }
                        moreLi.remove();
                    });
                    moreLi.appendChild(moreBtn);
                    childUl.appendChild(moreLi);
                }
            }

            // Toggle click
            toggle.addEventListener('click', function (e) {
                e.stopPropagation();
                var isOpen = childUl.style.display !== 'none';
                childUl.style.display = isOpen ? 'none' : '';
                toggle.textContent = isOpen ? '\u25B6' : '\u25BC';
                previewSpan.style.display = isOpen ? '' : 'none';
            });

            // Sync initial state
            previewSpan.style.display = depth < maxDepth ? 'none' : '';

        } else {
            // Leaf / primitive
            var spacer = document.createElement('span');
            spacer.className = 'jt-spacer';
            row.appendChild(spacer);

            if (key !== null) {
                var keySpan2 = document.createElement('span');
                keySpan2.className = 'jt-key';
                keySpan2.textContent = '"' + key + '"';
                row.appendChild(keySpan2);

                var colon2 = document.createElement('span');
                colon2.className = 'jt-colon';
                colon2.textContent = ': ';
                row.appendChild(colon2);
            }

            row.appendChild(createValueSpan(val));
        }

        return li;
    }

    // --- tree-level operations ---------------------------------------------

    function expandAll(container) {
        container.querySelectorAll('.jt-children').forEach(function (ul) {
            ul.style.display = '';
        });
        container.querySelectorAll('.jt-toggle').forEach(function (t) {
            t.textContent = '\u25BC';
        });
        container.querySelectorAll('.jt-preview').forEach(function (p) {
            p.style.display = 'none';
        });
    }

    function collapseAll(container) {
        container.querySelectorAll('.jt-children').forEach(function (ul) {
            ul.style.display = 'none';
        });
        container.querySelectorAll('.jt-toggle').forEach(function (t) {
            t.textContent = '\u25B6';
        });
        container.querySelectorAll('.jt-preview').forEach(function (p) {
            p.style.display = '';
        });
    }

    // Expose for toolbar button handlers
    window.jsonTreeExpandAll = expandAll;
    window.jsonTreeCollapseAll = collapseAll;

    // --- initialization ----------------------------------------------------

    function initTree(container) {
        var dataId = container.id + '-data';
        var dataScript = document.getElementById(dataId);
        if (!dataScript) return;

        var data;
        try {
            data = JSON.parse(dataScript.textContent);
        } catch (e) {
            container.textContent = 'Error parsing JSON: ' + e.message;
            return;
        }

        var maxDepth = parseInt(container.getAttribute('data-depth') || DEFAULT_DEPTH, 10);

        var ul = document.createElement('ul');
        ul.className = 'jt-root';

        var type = getType(data);
        if (type === 'object' || type === 'array') {
            // Render top-level node without a key
            ul.appendChild(buildNode(null, data, 0, maxDepth));
        } else {
            // Primitive at root
            var li = document.createElement('li');
            li.className = 'jt-node';
            var row = document.createElement('div');
            row.className = 'jt-row';
            row.appendChild(createValueSpan(data));
            li.appendChild(row);
            ul.appendChild(li);
        }

        container.appendChild(ul);
    }

    function initAllTrees() {
        document.querySelectorAll('.json-tree').forEach(initTree);
    }

    // Run on DOMContentLoaded or immediately if already loaded
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initAllTrees);
    } else {
        initAllTrees();
    }
})();
