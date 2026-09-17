const csrf = document.querySelector('meta[name="csrf-token"]')?.content;
const list = document.querySelector('#file-list');
const notice = document.querySelector('#notice');
const breadcrumbsEl = document.querySelector('#breadcrumbs');
const searchInput = document.querySelector('#search');
const driveActionBar = document.querySelector('#drive-action-bar');
const trashActionBar = document.querySelector('#trash-action-bar');
const uploadFab = document.querySelector('#upload-toggle');
const uploadMenu = document.querySelector('#upload-menu');
const storageText = document.querySelector('#storage-text');
const storageBar = document.querySelector('#storage-bar');
const sidebarStorage = document.querySelector('#sidebar-storage');
const infoModal = document.querySelector('#info-modal');
const storageModal = document.querySelector('#storage-modal');
const accountModal = document.querySelector('#account-modal');
const accountBtnDesktop = document.querySelector('#account-btn-desktop');
const accountBtnMobile = document.querySelector('#account-btn-mobile');
const accountCloseBtn = document.querySelector('#account-close');
const accountNotice = document.querySelector('#account-notice');
const accountTabUsername = document.querySelector('#account-tab-username');
const accountTabPassword = document.querySelector('#account-tab-password');
const accountTabUsers = document.querySelector('#account-tab-users');
const formChangeUsername = document.querySelector('#form-change-username');
const formChangePassword = document.querySelector('#form-change-password');
const formAddUser = document.querySelector('#form-add-user');
const accountCurrentName = document.querySelector('#account-current-name');
const accountCurrentUsername = document.querySelector('#account-current-username');
const accountBadgeRole = document.querySelector('#account-badge-role');
const settingsNewUsername = document.querySelector('#settings-new-username');
const settingsCurrentPassword = document.querySelector('#settings-current-password');
const settingsNewPassword = document.querySelector('#settings-new-password');
const settingsConfirmPassword = document.querySelector('#settings-confirm-password');
const newuserName = document.querySelector('#newuser-name');
const newuserUsername = document.querySelector('#newuser-username');
const newuserPassword = document.querySelector('#newuser-password');
const newuserIsAdmin = document.querySelector('#newuser-is-admin');
const previewModal = document.querySelector('#preview-modal');
const previewTitle = document.querySelector('#preview-title');
const previewIcon = document.querySelector('#preview-icon');
const previewDownloadBtn = document.querySelector('#preview-download-btn');
const previewBody = document.querySelector('#preview-body');

// Selection set and application state
const selection = new Set();
let currentView = 'drive'; // 'drive' | 'trash'
let currentParentId = '';
let nodes = [];
let breadcrumbs = [];

function showNotice(message, error = false) {
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
    if (/\.(zip|tar|gz|rar|7z)$/.test(name)) return 'zip.svg';
    if (/\.(mp3|wav|ogg|m4a|flac)$/.test(name)) return 'audio.svg';
    if (/\.(mp4|mov|webm|mkv|avi)$/.test(name)) return 'video.svg';
    if (/\.(png|jpe?g|gif|webp|svg|ico)$/.test(name)) return 'image.svg';
    return 'files.svg';
}

async function api(url, options = {}) {
    const headers = {
        'X-CSRF-Token': csrf,
        ...options.headers,
    };
    const response = await fetch(url, { ...options, headers });
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

async function updateStorageQuota() {
    try {
        const me = await api('/api/me');
        if (me) {
            const detectionAvailable = me.storage_detection_available !== false && me.filesystem_total_bytes !== null && me.filesystem_total_bytes !== undefined;
            const appUsed = me.alphadrive_used_bytes !== undefined ? me.alphadrive_used_bytes : (me.used_bytes || 0);

            const modalTotal = document.querySelector('#storage-modal-total');
            const modalUsed = document.querySelector('#storage-modal-used');
            const modalFree = document.querySelector('#storage-modal-free');
            const modalApp = document.querySelector('#storage-modal-app');

            if (detectionAvailable) {
                const fsTotal = me.filesystem_total_bytes || me.server_total || 0;
                const fsUsed = me.filesystem_used_bytes || me.server_used || 0;
                const fsFree = me.filesystem_free_bytes !== undefined && me.filesystem_free_bytes !== null ? me.filesystem_free_bytes : (fsTotal > fsUsed ? fsTotal - fsUsed : 0);

                const pct = fsTotal > 0 ? Math.min(100, Math.round((fsUsed / fsTotal) * 100)) : 0;
                if (storageText) {
                    storageText.textContent = `${pct}% of ${formatBytes(fsTotal)} Used`;
                }
                if (storageBar) {
                    storageBar.style.width = `${pct}%`;
                }

                if (modalTotal) modalTotal.textContent = formatBytes(fsTotal);
                if (modalUsed) modalUsed.textContent = formatBytes(fsUsed);
                if (modalFree) modalFree.textContent = formatBytes(fsFree);
            } else {
                if (storageText) {
                    storageText.textContent = 'Storage information unavailable';
                }
                if (storageBar) {
                    storageBar.style.width = '0%';
                }

                if (modalTotal) modalTotal.textContent = 'Unavailable';
                if (modalUsed) modalUsed.textContent = 'Unavailable';
                if (modalFree) modalFree.textContent = 'Unavailable';
            }

            if (modalApp) modalApp.textContent = formatBytes(appUsed);
        }
    } catch (e) {
        console.warn('Could not update storage quota', e);
        if (storageText) storageText.textContent = 'Storage information unavailable';
    }
}

function renderBreadcrumbs() {
    const downloadAllBtn = document.querySelector('#download-all-btn');
    if (currentView === 'trash') {
        if (breadcrumbsEl) breadcrumbsEl.innerHTML = '<span class="breadcrumb-btn active">Trash</span>';
        if (downloadAllBtn) downloadAllBtn.hidden = true;
        return;
    }

    if (!breadcrumbs || breadcrumbs.length <= 1) {
        if (breadcrumbsEl) breadcrumbsEl.innerHTML = '';
        if (downloadAllBtn) downloadAllBtn.hidden = true;
        return;
    }

    if (downloadAllBtn) {
        downloadAllBtn.hidden = nodes.length === 0;
    }

    const html = breadcrumbs.map((crumb, idx) => {
        const isLast = idx === breadcrumbs.length - 1;
        if (isLast) {
            return `<span class="breadcrumb-btn active">${esc(crumb.name)}</span>`;
        }
        return `<button class="breadcrumb-btn" type="button" data-crumb-id="${crumb.id}">${esc(crumb.name)}</button>`;
    }).join('<span class="breadcrumb-sep">&gt;</span>');

    if (breadcrumbsEl) breadcrumbsEl.innerHTML = html;
}

function updateSelectionBar() {
    const count = selection.size;

    document.querySelectorAll('.file-card').forEach(card => {
        card.classList.toggle('selected', selection.has(card.dataset.id));
    });

    if (currentView === 'drive') {
        if (driveActionBar) {
            driveActionBar.classList.add('active-view');
            driveActionBar.classList.toggle('visible', count > 0);
        }
        if (trashActionBar) {
            trashActionBar.classList.remove('active-view', 'visible');
        }
    } else {
        if (trashActionBar) {
            trashActionBar.classList.add('active-view');
            trashActionBar.classList.toggle('visible', count > 0);
        }
        if (driveActionBar) {
            driveActionBar.classList.remove('active-view', 'visible');
        }
    }

    const driveInfoBtn = document.querySelector('#info-selected');
    const trashInfoBtn = document.querySelector('#trash-info-selected');
    if (driveInfoBtn) driveInfoBtn.disabled = count !== 1;
    if (trashInfoBtn) trashInfoBtn.disabled = count !== 1;
}

function renderGrid() {
    const query = (searchInput ? searchInput.value : '').trim().toLowerCase();
    const visible = nodes.filter(node => (node.name || '').toLowerCase().includes(query));

    if (!visible.length) {
        list.innerHTML = query
            ? '<p class="grid-status">No items match your search.</p>'
            : (currentView === 'trash' ? '<p class="grid-status">Trash is empty.</p>' : '<p class="grid-status">This folder is empty.</p>');
        updateSelectionBar();
        return;
    }

    list.innerHTML = visible.map(node => `
        <button class="file-card ${selection.has(node.id) ? 'selected' : ''}" type="button" data-id="${node.id}" data-kind="${node.kind}">
            <span class="file-tile">
                <img src="/static/images/${iconFor(node)}" alt="">
            </span>
            <span class="file-card-name" title="${esc(node.name)}">${esc(node.name)}</span>
        </button>
    `).join('');

    updateSelectionBar();
}

async function loadFolder(id = '') {
    currentView = 'drive';
    currentParentId = id;
    selection.clear();
    updateNavState();
    if (uploadFab) uploadFab.hidden = false;
    list.innerHTML = '<p class="grid-status">Loading files…</p>';

    try {
        const url = `/api/nodes${id ? `?parent_id=${encodeURIComponent(id)}` : ''}`;
        const data = await api(url);
        nodes = data.nodes || [];
        breadcrumbs = data.breadcrumbs || [];
        renderBreadcrumbs();
        renderGrid();
        updateStorageQuota();
    } catch (error) {
        showNotice(error.message, true);
        list.innerHTML = '<p class="grid-status">Unable to load files.</p>';
    }
}

async function loadTrash() {
    currentView = 'trash';
    selection.clear();
    updateNavState();
    if (uploadFab) uploadFab.hidden = true;
    if (uploadMenu) uploadMenu.hidden = true;
    renderBreadcrumbs();
    list.innerHTML = '<p class="grid-status">Loading trash…</p>';

    try {
        const data = await api('/api/trash');
        nodes = data.nodes || [];
        renderGrid();
        updateStorageQuota();
    } catch (error) {
        showNotice(error.message, true);
        list.innerHTML = '<p class="grid-status">Unable to load trash.</p>';
    }
}

function updateNavState() {
    const isDrive = currentView === 'drive';
    document.querySelector('#nav-drive')?.classList.toggle('active', isDrive);
    document.querySelector('#nav-trash')?.classList.toggle('active', !isDrive);
    document.querySelector('#mobile-nav-drive')?.classList.toggle('active', isDrive);
    document.querySelector('#mobile-nav-trash')?.classList.toggle('active', !isDrive);
    updateSelectionBar();
}

// Nav clicks
document.querySelector('#nav-drive')?.addEventListener('click', () => loadFolder(''));
document.querySelector('#nav-trash')?.addEventListener('click', () => loadTrash());
document.querySelector('#mobile-nav-drive')?.addEventListener('click', () => loadFolder(''));
document.querySelector('#mobile-nav-trash')?.addEventListener('click', () => loadTrash());

// Breadcrumb clicks
breadcrumbsEl?.addEventListener('click', event => {
    const btn = event.target.closest('[data-crumb-id]');
    if (!btn) return;
    loadFolder(btn.dataset.crumbId);
});

// File list interactions
list.addEventListener('click', event => {
    const card = event.target.closest('[data-id]');
    if (!card) return;
    const node = nodes.find(item => item.id === card.dataset.id);
    if (!node) return;

    if (event.detail > 1) {
        if (node.kind === 'folder') {
            if (currentView === 'drive') {
                loadFolder(node.id);
            } else {
                showInfo(node);
            }
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

// Search
searchInput?.addEventListener('input', renderGrid);

// Deselect when clicking empty space
document.addEventListener('click', event => {
    if (!selection.size) return;
    const isCard = event.target.closest('.file-card');
    const isBar = event.target.closest('.bottom-action-bar');
    const isMenu = event.target.closest('.upload-menu-popup');
    const isFab = event.target.closest('#upload-toggle');
    const isDialog = event.target.closest('#info-modal') || event.target.closest('#storage-modal') || event.target.closest('#preview-modal');
    if (!isCard && !isBar && !isMenu && !isFab && !isDialog) {
        selection.clear();
        updateSelectionBar();
    }
});

// Create folder
document.querySelector('#new-folder')?.addEventListener('click', async () => {
    const name = window.prompt('Folder name:');
    if (!name || !name.trim()) return;
    try {
        await api('/api/folders', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ parent_id: currentParentId, name: name.trim() }),
        });
        if (uploadMenu) uploadMenu.hidden = true;
        showNotice(`Created folder "${name.trim()}"`);
        loadFolder(currentParentId);
    } catch (error) {
        showNotice(error.message, true);
    }
});

// Upload FAB toggle
uploadFab?.addEventListener('click', () => {
    if (uploadMenu) uploadMenu.hidden = !uploadMenu.hidden;
});

// File upload
document.querySelector('#upload')?.addEventListener('change', async event => {
    const filesToUpload = Array.from(event.target.files);
    if (!filesToUpload.length) return;

    let successCount = 0;
    for (const file of filesToUpload) {
        const form = new FormData();
        form.set('parent_id', currentParentId);
        form.set('file', file);
        try {
            await api('/api/uploads', { method: 'POST', body: form });
            successCount++;
        } catch (error) {
            showNotice(`Failed to upload ${file.name}: ${error.message}`, true);
        }
    }
    event.target.value = '';
    if (uploadMenu) uploadMenu.hidden = true;
    if (successCount > 0) {
        showNotice(`Uploaded ${successCount} file(s)`);
    }
    loadFolder(currentParentId);
});

// Folder upload
document.querySelector('#upload-folder')?.addEventListener('change', async event => {
    const filesToUpload = Array.from(event.target.files);
    if (!filesToUpload.length) return;

    let successCount = 0;
    for (const file of filesToUpload) {
        const form = new FormData();
        form.set('parent_id', currentParentId);
        form.set('file', file);
        try {
            await api('/api/uploads', { method: 'POST', body: form });
            successCount++;
        } catch (error) {
            showNotice(`Failed to upload ${file.name}: ${error.message}`, true);
        }
    }
    event.target.value = '';
    if (uploadMenu) uploadMenu.hidden = true;
    if (successCount > 0) {
        showNotice(`Uploaded ${successCount} file(s) from folder`);
    }
    loadFolder(currentParentId);
});

async function downloadNodes(ids) {
    if (!ids || ids.length === 0) return;
    if (ids.length === 1) {
        const node = nodes.find(n => n.id === ids[0]);
        if (node && node.kind === 'file') {
            window.location = `/api/files/${encodeURIComponent(node.id)}/download`;
            return;
        }
    }

    try {
        showNotice('Preparing download archive…');
        const response = await fetch('/api/nodes/download', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-Token': csrf,
            },
            body: JSON.stringify({ ids }),
        });
        if (!response.ok) {
            throw new Error('Download archive request failed');
        }
        const blob = await response.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        const disp = response.headers.get('content-disposition') || '';
        const match = disp.match(/filename="?([^"]+)"?/);
        a.download = match ? match[1] : 'alphadrive-archive.zip';
        document.body.appendChild(a);
        a.click();
        a.remove();
        window.URL.revokeObjectURL(url);
    } catch (error) {
        showNotice(error.message, true);
    }
}

// Download selected
document.querySelector('#download-selected')?.addEventListener('click', () => {
    if (selection.size === 0) return;
    downloadNodes(Array.from(selection));
});

// Download all in current folder
document.querySelector('#download-all-btn')?.addEventListener('click', () => {
    if (!nodes.length) return;
    downloadNodes(nodes.map(n => n.id));
});

// Trash selected
document.querySelector('#trash-selected')?.addEventListener('click', async () => {
    if (selection.size === 0) return;
    const ids = Array.from(selection);
    try {
        await api('/api/nodes/trash', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ids }),
        });
        showNotice(`Moved ${ids.length} item(s) to trash`);
        loadFolder(currentParentId);
    } catch (error) {
        showNotice(error.message, true);
    }
});

// Restore selected
document.querySelector('#restore-selected')?.addEventListener('click', async () => {
    if (selection.size === 0) return;
    const ids = Array.from(selection);
    try {
        await api('/api/nodes/restore', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ids }),
        });
        showNotice(`Restored ${ids.length} item(s)`);
        loadTrash();
    } catch (error) {
        showNotice(error.message, true);
    }
});

// Permanent delete selected
document.querySelector('#delete-selected')?.addEventListener('click', async () => {
    if (selection.size === 0) return;
    const ids = Array.from(selection);
    if (!window.confirm(`Permanently delete ${ids.length} item(s)? This cannot be undone.`)) {
        return;
    }
    try {
        await api('/api/nodes', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ids }),
        });
        showNotice(`Permanently deleted ${ids.length} item(s)`);
        loadTrash();
    } catch (error) {
        showNotice(error.message, true);
    }
});

// File information modal
function showInfo(node) {
    if (!node || !infoModal) return;
    document.querySelector('#info-title').textContent = node.kind === 'folder' ? 'Folder details' : 'File details';
    document.querySelector('#info-name').textContent = node.name || '-';
    document.querySelector('#info-kind').textContent = node.kind === 'folder' ? 'Folder' : 'File';
    document.querySelector('#info-mime').textContent = node.kind === 'folder' ? 'directory' : (node.mime_type || 'application/octet-stream');
    document.querySelector('#info-size').textContent = node.kind === 'folder' ? '-' : `${formatBytes(node.size_bytes)} (${(node.size_bytes || 0).toLocaleString()} bytes)`;
    document.querySelector('#info-created').textContent = formatDate(node.created_at);
    document.querySelector('#info-updated').textContent = formatDate(node.updated_at);
    document.querySelector('#info-status').textContent = node.trashed_at ? `In trash (since ${formatDate(node.trashed_at)})` : 'Active';

    infoModal.showModal();
}

async function openSelectedInfo() {
    if (selection.size !== 1) return;
    const id = Array.from(selection)[0];
    let node = nodes.find(n => n.id === id);
    if (!node) {
        try {
            node = await api(`/api/nodes/${encodeURIComponent(id)}`);
        } catch (error) {
            showNotice(error.message, true);
            return;
        }
    }
    showInfo(node);
}

document.querySelector('#info-selected')?.addEventListener('click', openSelectedInfo);
document.querySelector('#trash-info-selected')?.addEventListener('click', openSelectedInfo);

document.querySelector('#info-close')?.addEventListener('click', () => {
    infoModal?.close();
});

infoModal?.addEventListener('click', event => {
    if (event.target === infoModal) {
        infoModal.close();
    }
});

// Share Modal implementation
const shareModal = document.querySelector('#share-modal');
const shareActiveState = document.querySelector('#share-active-state');
const shareCreateForm = document.querySelector('#share-create-form');
const shareLinkInput = document.querySelector('#share-link-input');
const shareActiveDetails = document.querySelector('#share-active-details');
const shareCopyBtn = document.querySelector('#share-copy-btn');
const shareOpenBtn = document.querySelector('#share-open-btn');
const shareRevokeBtn = document.querySelector('#share-revoke-btn');
const shareNodeIdInput = document.querySelector('#share-node-id');
const shareCustomSlugInput = document.querySelector('#share-custom-slug');
const shareNewPasswordInput = document.querySelector('#share-new-password');
const shareNewExpirySelect = document.querySelector('#share-new-expiry');

async function openSelectedShare() {
    if (selection.size !== 1) return;
    const id = Array.from(selection)[0];
    const node = nodes.find(n => n.id === id);
    const modalTitle = document.querySelector('#share-modal-title');
    if (modalTitle) {
        modalTitle.textContent = node ? `Share "${node.name}"` : 'Share item';
    }
    if (shareNodeIdInput) {
        shareNodeIdInput.value = id;
    }

    // Check if an active share already exists
    try {
        const share = await api(`/api/nodes/${encodeURIComponent(id)}/share`);
        if (share && share.slug) {
            if (shareActiveState) shareActiveState.hidden = false;
            if (shareCreateForm) shareCreateForm.hidden = true;
            if (shareLinkInput) shareLinkInput.value = share.public_url || `${window.location.origin}/s/${share.slug}`;
            if (shareActiveDetails) {
                const parts = [
                    `Views: ${share.view_count || 0}`,
                    share.expires_at ? `Expires: ${formatDate(share.expires_at)}` : 'Never expires',
                ];
                if (share.has_password) parts.push('Password protected');
                shareActiveDetails.textContent = parts.join(' • ');
            }
            if (shareRevokeBtn) shareRevokeBtn.dataset.shareId = share.id;
            if (shareOpenBtn) {
                shareOpenBtn.onclick = () => window.open(share.public_url || `/s/${share.slug}`, '_blank');
            }
        }
    } catch (e) {
        // No active share found, show create form
        if (shareActiveState) shareActiveState.hidden = true;
        if (shareCreateForm) {
            shareCreateForm.hidden = false;
            if (shareCustomSlugInput) shareCustomSlugInput.value = '';
            if (shareNewPasswordInput) shareNewPasswordInput.value = '';
            if (shareNewExpirySelect) shareNewExpirySelect.value = '';
        }
    }

    shareModal?.showModal();
}

document.querySelector('#link-selected')?.addEventListener('click', openSelectedShare);

document.querySelector('#share-close')?.addEventListener('click', () => {
    shareModal?.close();
});

shareModal?.addEventListener('click', event => {
    if (event.target === shareModal) {
        shareModal.close();
    }
});

shareCreateForm?.addEventListener('submit', async event => {
    event.preventDefault();
    const nodeId = shareNodeIdInput?.value;
    const customSlug = shareCustomSlugInput?.value?.trim() || undefined;
    const password = shareNewPasswordInput?.value || undefined;
    const expiresIn = shareNewExpirySelect?.value || undefined;

    if (password && password.length < 12) {
        showNotice('Share password must be at least 12 characters', true);
        return;
    }

    try {
        const result = await api('/api/shares', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                node_id: nodeId,
                custom_slug: customSlug,
                password: password,
                expires_in: expiresIn,
            }),
        });

        if (shareActiveState) shareActiveState.hidden = false;
        if (shareCreateForm) shareCreateForm.hidden = true;
        if (shareLinkInput) shareLinkInput.value = result.public_url || `${window.location.origin}/s/${result.slug}`;
        if (shareActiveDetails) {
            const parts = [
                'Views: 0',
                result.expires_at ? `Expires: ${formatDate(result.expires_at)}` : 'Never expires',
            ];
            if (result.has_password) parts.push('Password protected');
            shareActiveDetails.textContent = parts.join(' • ');
        }
        if (shareRevokeBtn) shareRevokeBtn.dataset.shareId = result.id;
        if (shareOpenBtn) {
            shareOpenBtn.onclick = () => window.open(result.public_url || `/s/${result.slug}`, '_blank');
        }

        showNotice('Public link created!');
        try {
            await navigator.clipboard.writeText(shareLinkInput.value);
            showNotice('Link created & copied to clipboard!');
        } catch {
            // Clipboard write might fail if permissions not granted
        }
    } catch (err) {
        showNotice(err.message, true);
    }
});

shareCopyBtn?.addEventListener('click', async () => {
    if (!shareLinkInput || !shareLinkInput.value) return;
    try {
        await navigator.clipboard.writeText(shareLinkInput.value);
        shareCopyBtn.textContent = 'Copied!';
        setTimeout(() => {
            shareCopyBtn.textContent = 'Copy';
        }, 2000);
    } catch {
        shareLinkInput.select();
        document.execCommand('copy');
        shareCopyBtn.textContent = 'Copied!';
        setTimeout(() => {
            shareCopyBtn.textContent = 'Copy';
        }, 2000);
    }
});

shareRevokeBtn?.addEventListener('click', async () => {
    const shareId = shareRevokeBtn.dataset.shareId;
    if (!shareId) return;
    if (!confirm('Revoke this public link? Anyone with the link will lose access immediately.')) {
        return;
    }

    try {
        await api(`/api/shares/${encodeURIComponent(shareId)}`, {
            method: 'DELETE',
        });
        showNotice('Public link revoked');
        if (shareActiveState) shareActiveState.hidden = true;
        if (shareCreateForm) {
            shareCreateForm.hidden = false;
            if (shareCustomSlugInput) shareCustomSlugInput.value = '';
            if (shareNewPasswordInput) shareNewPasswordInput.value = '';
            if (shareNewExpirySelect) shareNewExpirySelect.value = '';
        }
    } catch (err) {
        showNotice(err.message, true);
    }
});

// Storage overview modal triggers
sidebarStorage?.addEventListener('click', () => {
    storageModal?.showModal();
});
sidebarStorage?.addEventListener('keydown', event => {
    if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        storageModal?.showModal();
    }
});

document.querySelector('#storage-close')?.addEventListener('click', () => {
    storageModal?.close();
});

// Account settings modal implementation
function showAccountNotice(message, isError = false) {
    if (!accountNotice) return;
    accountNotice.hidden = false;
    accountNotice.textContent = message;
    accountNotice.className = isError ? 'account-notice notice-error' : 'account-notice notice-success';
}

function setAccountTab(tab) {
    if (!accountModal) return;
    if (accountNotice) {
        accountNotice.hidden = true;
        accountNotice.textContent = '';
    }

    accountTabUsername?.classList.toggle('active', tab === 'username');
    accountTabPassword?.classList.toggle('active', tab === 'password');
    accountTabUsers?.classList.toggle('active', tab === 'users');

    let activeForm = null;
    if (formChangeUsername) {
        formChangeUsername.hidden = tab !== 'username';
        if (tab === 'username') activeForm = formChangeUsername;
    }
    if (formChangePassword) {
        formChangePassword.hidden = tab !== 'password';
        if (tab === 'password') activeForm = formChangePassword;
    }
    if (formAddUser) {
        formAddUser.hidden = tab !== 'users';
        if (tab === 'users') activeForm = formAddUser;
    }

    if (activeForm) {
        activeForm.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        const firstInput = activeForm.querySelector('input:not([type="hidden"]):not([disabled])');
        if (firstInput) {
            setTimeout(() => firstInput.focus(), 50);
        }
    }
}

async function openAccountModal() {
    if (!accountModal) return;
    accountModal.showModal();
    setAccountTab('username');
    try {
        const me = await api('/api/me');
        if (me) {
            if (accountCurrentName) accountCurrentName.textContent = me.name || me.username;
            if (accountCurrentUsername) accountCurrentUsername.textContent = `@${me.username}`;
            if (accountBadgeRole) accountBadgeRole.textContent = me.is_admin ? 'Admin' : 'User';
            if (accountTabUsers) accountTabUsers.hidden = !me.is_admin;
            if (settingsNewUsername) settingsNewUsername.value = me.username;
        }
    } catch (e) {
        console.warn('Could not fetch user details', e);
    }
}

accountBtnDesktop?.addEventListener('click', openAccountModal);
accountBtnMobile?.addEventListener('click', openAccountModal);
accountCloseBtn?.addEventListener('click', () => accountModal?.close());
accountModal?.addEventListener('click', event => {
    if (event.target === accountModal) {
        accountModal.close();
    }
});

accountTabUsername?.addEventListener('click', () => setAccountTab('username'));
accountTabPassword?.addEventListener('click', () => setAccountTab('password'));
accountTabUsers?.addEventListener('click', () => setAccountTab('users'));

formChangeUsername?.addEventListener('submit', async event => {
    event.preventDefault();
    const newUsername = (settingsNewUsername?.value || '').trim();
    if (!newUsername) return;

    try {
        const res = await api('/api/account/username', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ new_username: newUsername }),
        });
        showAccountNotice(res.message || 'Username updated successfully');
        if (accountCurrentUsername) accountCurrentUsername.textContent = `@${res.username}`;
        if (accountBtnDesktop) accountBtnDesktop.title = res.username;
    } catch (err) {
        showAccountNotice(err.message, true);
    }
});

formChangePassword?.addEventListener('submit', async event => {
    event.preventDefault();
    const currentPassword = settingsCurrentPassword?.value || '';
    const newPassword = settingsNewPassword?.value || '';
    const confirmPassword = settingsConfirmPassword?.value || '';

    if (!currentPassword) {
        showAccountNotice('Current password is required', true);
        return;
    }
    if (newPassword.length < 12) {
        showAccountNotice('New password must be at least 12 characters', true);
        return;
    }
    if (newPassword !== confirmPassword) {
        showAccountNotice('New passwords do not match', true);
        return;
    }

    try {
        const res = await api('/api/account/password', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                current_password: currentPassword,
                new_password: newPassword,
            }),
        });
        showAccountNotice(res.message || 'Password changed successfully');
        if (settingsCurrentPassword) settingsCurrentPassword.value = '';
        if (settingsNewPassword) settingsNewPassword.value = '';
        if (settingsConfirmPassword) settingsConfirmPassword.value = '';
    } catch (err) {
        showAccountNotice(err.message, true);
    }
});

formAddUser?.addEventListener('submit', async event => {
    event.preventDefault();
    const username = (newuserUsername?.value || '').trim();
    const name = (newuserName?.value || '').trim();
    const password = newuserPassword?.value || '';
    const isAdmin = !!newuserIsAdmin?.checked;

    if (!username) {
        showAccountNotice('Username is required', true);
        return;
    }
    if (password.length < 12) {
        showAccountNotice('Password must be at least 12 characters', true);
        return;
    }

    try {
        const res = await api('/api/account/users', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                username,
                name,
                password,
                is_admin: isAdmin,
            }),
        });
        showAccountNotice(res.message || 'User created successfully');
        if (newuserUsername) newuserUsername.value = '';
        if (newuserName) newuserName.value = '';
        if (newuserPassword) newuserPassword.value = '';
        if (newuserIsAdmin) newuserIsAdmin.checked = false;
    } catch (err) {
        showAccountNotice(err.message, true);
    }
});

// File preview implementation
async function openPreview(node) {
    if (!node || !previewModal) return;
    if (previewTitle) previewTitle.textContent = node.name || 'File preview';
    if (previewIcon) previewIcon.src = `/static/images/${iconFor(node)}`;
    if (previewDownloadBtn) {
        previewDownloadBtn.href = `/api/files/${encodeURIComponent(node.id)}/download`;
        previewDownloadBtn.setAttribute('download', node.name || 'download');
    }

    const viewUrl = `/api/files/${encodeURIComponent(node.id)}/view`;
    const mime = (node.mime_type || '').toLowerCase();
    const name = (node.name || '').toLowerCase();

    if (previewBody) {
        previewBody.innerHTML = '<p class="grid-status">Loading preview…</p>';
    }
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
                <p class="preview-unsupported-hint">Preview is not available for this file format.</p>
                <a href="/api/files/${encodeURIComponent(node.id)}/download" download="${esc(node.name)}" class="btn-submit preview-download-cta">Download File</a>
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
    if (event.target === previewModal) {
        closePreview();
    }
});

// Initial boot
loadFolder();

