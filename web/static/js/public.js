const slug = window.ALPHADRIVE_SHARE_SLUG;
const rootID = window.ALPHADRIVE_SHARE_ROOT;

const list = document.querySelector('#public-file-list');
const notice = document.querySelector('#notice');
const breadcrumbsEl = document.querySelector('#public-breadcrumbs');
const selectionBar = document.querySelector('#public-selection-bar');
const infoModal = document.querySelector('#info-modal');
const previewModal = document.querySelector('#preview-modal');
const previewTitle = document.querySelector('#preview-title');
const previewIcon = document.querySelector('#preview-icon');
const previewDownloadBtn = document.querySelector('#preview-download-btn');
const previewBody = document.querySelector('#preview-body');

const selection = new Set();
let currentFolderId = rootID;
let nodes = [];
let breadcrumbs = [];

function showNotice(message, error = false) {
    if (!notice) return;
    notice.hidden = false;
    notice.textContent = message;
    notice.style.color = error ? '#ffc2c2' : '#cceffe';
    setTimeout(() => {
        if (notice.textContent === message) {
            notice.hidden = true;
        }
    }, 5000);
}

function esc(value) {
    const el = document.createElement('span');
    el.textContent = value || '';
    return el.innerHTML;
}

function formatBytes(bytes) {
    if (bytes === undefined || bytes === null || isNaN(bytes)) return '0 B';
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    const val = (bytes / Math.pow(k, i)).toFixed(i === 0 ? 0 : 1);
    const cleanVal = val.endsWith('.0') ? val.slice(0, -2) : val;
    return `${cleanVal} ${sizes[i]}`;
}

function formatDate(dateStr) {
    if (!dateStr) return '-';
    try {
        const d = new Date(dateStr);
        return d.toLocaleString();
    } catch {
        return dateStr;
    }
}

function iconFor(node) {
    if (node.kind === 'folder') return 'folder.svg';
    const name = (node.name || '').toLowerCase();
    if (name.endsWith('.pdf') || node.mime_type === 'application/pdf') return 'pdf.svg';
    if (/\.(zip|tar|gz|rar|7z)$/.test(name)) return 'zip.svg';
    if (/\.(mp3|wav|ogg|m4a|flac)$/.test(name)) return 'audio.svg';
    if (/\.(mp4|mov|webm|mkv|avi)$/.test(name)) return 'video.svg';
    if (/\.(png|jpe?g|gif|webp|svg|ico)$/.test(name)) return 'image.svg';
    return 'files.svg';
}

async function api(url, options = {}) {
    const response = await fetch(url, options);
    if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(body.error?.message || `Request failed (${response.status})`);
    }
    const contentType = response.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
        return response.json();
    }
    return response;
}

function renderBreadcrumbs() {
    if (!breadcrumbsEl) return;
    if (!breadcrumbs || breadcrumbs.length <= 1) {
        breadcrumbsEl.innerHTML = breadcrumbs.length === 1 ? `<span class="breadcrumb-btn active">${esc(breadcrumbs[0].name)}</span>` : '';
        return;
    }

    const html = breadcrumbs.map((crumb, idx) => {
        const isLast = idx === breadcrumbs.length - 1;
        if (isLast) {
            return `<span class="breadcrumb-btn active">${esc(crumb.name)}</span>`;
        }
        return `<button class="breadcrumb-btn" type="button" data-crumb-id="${crumb.id}">${esc(crumb.name)}</button>`;
    }).join('<span class="breadcrumb-sep">&gt;</span>');

    breadcrumbsEl.innerHTML = html;
}

function updateSelectionBar() {
    const count = selection.size;
    if (selectionBar) {
        selectionBar.classList.add('active-view');
        selectionBar.classList.toggle('visible', count > 0);
    }
    document.querySelectorAll('.file-card').forEach(card => {
        card.classList.toggle('selected', selection.has(card.dataset.id));
    });

    const infoBtn = document.querySelector('#public-info-selected');
    if (infoBtn) {
        infoBtn.disabled = count !== 1;
    }
}

function renderGrid() {
    if (!list) return;
    if (!nodes.length) {
        list.innerHTML = '<p class="grid-status">This folder is empty.</p>';
        updateSelectionBar();
        return;
    }

    list.innerHTML = nodes.map(node => `
        <button class="file-card ${selection.has(node.id) ? 'selected' : ''}" type="button" data-id="${node.id}" data-kind="${node.kind}">
            <span class="file-tile">
                <img src="/static/images/${iconFor(node)}" alt="">
            </span>
            <span class="file-card-name" title="${esc(node.name)}">${esc(node.name)}</span>
        </button>
    `).join('');

    updateSelectionBar();
}

async function loadFolder(id = rootID) {
    currentFolderId = id;
    selection.clear();
    if (list) list.innerHTML = '<p class="grid-status">Loading files…</p>';

    try {
        const url = `/s/${encodeURIComponent(slug)}/nodes?folder_id=${encodeURIComponent(id)}`;
        const data = await api(url);
        nodes = data.nodes || [];
        breadcrumbs = data.breadcrumbs || [];
        renderBreadcrumbs();
        renderGrid();
    } catch (error) {
        showNotice(error.message, true);
        if (list) list.innerHTML = '<p class="grid-status">Unable to load shared files.</p>';
    }
}

// Breadcrumbs click
breadcrumbsEl?.addEventListener('click', event => {
    const btn = event.target.closest('[data-crumb-id]');
    if (!btn) return;
    loadFolder(btn.dataset.crumbId);
});

// File list interaction
list?.addEventListener('click', event => {
    const card = event.target.closest('[data-id]');
    if (!card) return;
    const node = nodes.find(item => item.id === card.dataset.id);
    if (!node) return;

    if (event.detail > 1) {
        if (node.kind === 'folder') {
            loadFolder(node.id);
        } else {
            openPreview(node);
        }
        return;
    }

    if (selection.has(node.id)) {
        selection.delete(node.id);
    } else {
        selection.add(node.id);
    }
    updateSelectionBar();
});

let lastDragEndTime = 0;

// Deselect on outside click
document.addEventListener('click', event => {
    if (!selection.size) return;
    if (Date.now() - lastDragEndTime < 200) {
        return;
    }
    const isCard = event.target.closest('.file-card');
    const isBar = event.target.closest('.bottom-action-bar');
    const isDialog = event.target.closest('#info-modal') || event.target.closest('#preview-modal');
    if (!isCard && !isBar && !isDialog) {
        selection.clear();
        updateSelectionBar();
    }
});

// Download All function (downloads the entire shared folder)
async function downloadAll() {
    try {
        showNotice('Preparing download archive…');
        const response = await fetch(`/s/${encodeURIComponent(slug)}/download`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder_id: rootID }),
        });
        if (!response.ok) throw new Error('Failed to generate archive');
        const blob = await response.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        const disp = response.headers.get('content-disposition') || '';
        const match = disp.match(/filename="?([^"]+)"?/);
        a.download = match ? match[1] : 'alphadrive-share.zip';
        document.body.appendChild(a);
        a.click();
        a.remove();
        window.URL.revokeObjectURL(url);
    } catch (e) {
        showNotice(e.message, true);
    }
}

document.querySelector('#public-download-all-btn')?.addEventListener('click', downloadAll);

// Download Selected
document.querySelector('#public-download-selected')?.addEventListener('click', () => {
    if (selection.size === 0) return;
    const ids = Array.from(selection);
    if (ids.length === 1) {
        const node = nodes.find(n => n.id === ids[0]);
        if (node && node.kind === 'file') {
            window.location = `/s/${encodeURIComponent(slug)}/files/${encodeURIComponent(node.id)}/download`;
            return;
        }
    }
    downloadAll();
});

// Info dialog
function showInfo(node) {
    if (!node || !infoModal) return;
    document.querySelector('#info-title').textContent = node.kind === 'folder' ? 'Folder details' : 'File details';
    document.querySelector('#info-name').textContent = node.name || '-';
    document.querySelector('#info-kind').textContent = node.kind === 'folder' ? 'Folder' : 'File';
    document.querySelector('#info-mime').textContent = node.kind === 'folder' ? 'directory' : (node.mime_type || 'application/octet-stream');
    document.querySelector('#info-size').textContent = node.kind === 'folder' ? '-' : formatBytes(node.size_bytes);
    document.querySelector('#info-updated').textContent = formatDate(node.updated_at);
    infoModal.showModal();
}

document.querySelector('#public-info-selected')?.addEventListener('click', () => {
    if (selection.size !== 1) return;
    const id = Array.from(selection)[0];
    const node = nodes.find(n => n.id === id);
    if (node) showInfo(node);
});

document.querySelector('#info-close')?.addEventListener('click', () => infoModal?.close());
infoModal?.addEventListener('click', event => {
    if (event.target === infoModal) infoModal.close();
});

// File Preview
async function openPreview(node) {
    if (!node || !previewModal) return;
    if (previewTitle) previewTitle.textContent = node.name || 'File preview';
    if (previewIcon) previewIcon.src = `/static/images/${iconFor(node)}`;
    if (previewDownloadBtn) {
        previewDownloadBtn.href = `/s/${encodeURIComponent(slug)}/files/${encodeURIComponent(node.id)}/download`;
        previewDownloadBtn.setAttribute('download', node.name || 'download');
    }

    const viewUrl = `/s/${encodeURIComponent(slug)}/files/${encodeURIComponent(node.id)}/view`;
    const mime = (node.mime_type || '').toLowerCase();
    const name = (node.name || '').toLowerCase();

    if (previewBody) previewBody.innerHTML = '<p class="grid-status">Loading preview…</p>';
    previewModal.showModal();

    if (mime.startsWith('image/') || /\.(png|jpe?g|gif|webp|svg|ico|bmp)$/.test(name)) {
        previewBody.innerHTML = `<img class="preview-media" src="${viewUrl}" alt="${esc(node.name)}">`;
    } else if (mime.startsWith('video/') || /\.(mp4|webm|mkv|mov|avi)$/.test(name)) {
        previewBody.innerHTML = `<video class="preview-video" controls autoplay playsinline src="${viewUrl}"></video>`;
    } else if (mime.startsWith('audio/') || /\.(mp3|wav|ogg|m4a|flac|aac)$/.test(name)) {
        previewBody.innerHTML = `
            <div class="preview-audio-container">
                <img src="/static/images/audio.svg" alt="" class="preview-audio-banner">
                <p class="preview-audio-name">${esc(node.name)}</p>
                <audio controls autoplay src="${viewUrl}" class="preview-audio-player"></audio>
            </div>
        `;
    } else if (mime === 'application/pdf' || name.endsWith('.pdf')) {
        previewBody.innerHTML = `<iframe class="preview-frame" src="${viewUrl}" title="${esc(node.name)}"></iframe>`;
    } else if (
        mime.startsWith('text/') ||
        mime.includes('json') ||
        mime.includes('javascript') ||
        mime.includes('xml') ||
        /\.(txt|md|json|js|ts|html|css|go|py|rs|c|cpp|h|sh|bat|ps1|yaml|yml|toml|env|sql|log|csv|tsv|ini|xml)$/.test(name)
    ) {
        try {
            const resp = await fetch(viewUrl);
            if (!resp.ok) throw new Error('Could not load text content');
            const text = await resp.text();
            const displayText = text.length > 500000 ? text.slice(0, 500000) + '\n\n… [Content truncated]' : text;
            previewBody.innerHTML = `<pre class="preview-code"><code>${esc(displayText)}</code></pre>`;
        } catch (e) {
            previewBody.innerHTML = `<p class="grid-status">Unable to display text preview: ${esc(e.message)}</p>`;
        }
    } else {
        previewBody.innerHTML = `
            <div class="preview-unsupported-box">
                <img src="/static/images/${iconFor(node)}" alt="" class="preview-unsupported-icon">
                <p class="preview-unsupported-name">${esc(node.name)}</p>
                <p class="preview-unsupported-size">${formatBytes(node.size_bytes)}</p>
                <a href="/s/${encodeURIComponent(slug)}/files/${encodeURIComponent(node.id)}/download" download="${esc(node.name)}" class="btn-submit preview-download-cta">Download File</a>
            </div>
        `;
    }
}

function closePreview() {
    if (!previewModal) return;
    const media = previewModal.querySelectorAll('video, audio');
    media.forEach(m => {
        m.pause();
        m.src = '';
    });
    previewModal.close();
}

document.querySelector('#preview-close')?.addEventListener('click', closePreview);
previewModal?.addEventListener('click', event => {
    if (event.target === previewModal) closePreview();
});

// Rectangular Click & Drag (Marquee / Lasso) Selection
function initMarqueeSelection() {
    const marquee = document.querySelector('#selection-marquee');
    if (!marquee) return;

    let isSelecting = false;
    let startX = 0;
    let startY = 0;
    let initialSelected = new Set();
    let isDragThresholdMet = false;

    document.addEventListener('mousedown', event => {
        if (event.button !== 0) return;

        const target = event.target;
        if (
            target.closest('.file-card') ||
            target.closest('.bottom-action-bar') ||
            target.closest('.public-header') ||
            target.closest('.info-dialog') ||
            target.closest('dialog') ||
            target.closest('button, input, select, textarea, a, form')
        ) {
            return;
        }

        isSelecting = true;
        isDragThresholdMet = false;
        startX = event.clientX;
        startY = event.clientY;

        if (event.shiftKey || event.ctrlKey || event.metaKey) {
            initialSelected = new Set(selection);
        } else {
            initialSelected = new Set();
            selection.clear();
            updateSelectionBar();
        }

        marquee.style.left = `${startX}px`;
        marquee.style.top = `${startY}px`;
        marquee.style.width = '0px';
        marquee.style.height = '0px';
    });

    window.addEventListener('mousemove', event => {
        if (!isSelecting) return;

        const currentX = event.clientX;
        const currentY = event.clientY;
        const deltaX = currentX - startX;
        const deltaY = currentY - startY;

        if (!isDragThresholdMet) {
            if (Math.hypot(deltaX, deltaY) > 5) {
                isDragThresholdMet = true;
                marquee.hidden = false;
                document.body.style.userSelect = 'none';
            } else {
                return;
            }
        }

        const rectLeft = Math.min(startX, currentX);
        const rectTop = Math.min(startY, currentY);
        const rectWidth = Math.abs(deltaX);
        const rectHeight = Math.abs(deltaY);
        const rectRight = rectLeft + rectWidth;
        const rectBottom = rectTop + rectHeight;

        marquee.style.left = `${rectLeft}px`;
        marquee.style.top = `${rectTop}px`;
        marquee.style.width = `${rectWidth}px`;
        marquee.style.height = `${rectHeight}px`;

        const cards = list ? list.querySelectorAll('.file-card') : [];
        const newlySelected = new Set(initialSelected);

        cards.forEach(card => {
            const cardRect = card.getBoundingClientRect();
            const intersects = !(
                cardRect.right < rectLeft ||
                cardRect.left > rectRight ||
                cardRect.bottom < rectTop ||
                cardRect.top > rectBottom
            );

            const cardId = card.dataset.id;
            if (intersects) {
                newlySelected.add(cardId);
            } else if (!initialSelected.has(cardId)) {
                newlySelected.delete(cardId);
            }
        });

        selection.clear();
        newlySelected.forEach(id => selection.add(id));

        cards.forEach(card => {
            card.classList.toggle('selected', selection.has(card.dataset.id));
        });

        updateSelectionBar();
    });

    window.addEventListener('mouseup', () => {
        if (!isSelecting) return;
        isSelecting = false;
        if (isDragThresholdMet) {
            lastDragEndTime = Date.now();
            marquee.hidden = true;
            document.body.style.userSelect = '';
        }
    });
}

// Initial boot
initMarqueeSelection();
loadFolder(rootID);

