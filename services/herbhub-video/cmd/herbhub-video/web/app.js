// HerbHub Video — Frontend Application
// Vanilla JS SPA for the herbhub-video web interface.
// Proxies all generation through the video-narrator API for full pipeline support.

const PHASES = ['preprocessing', 'submitting', 'generating', 'downloading', 'stitching'];

const PHASE_LABELS = {
    queued: 'Queued',
    preprocessing: 'TTS Preprocessing',
    submitting: 'Submitting to MuseTalk',
    generating: 'Generating Video',
    downloading: 'Downloading MP4',
    stitching: 'Stitching (ffmpeg)',
    completed: 'Completed',
    failed: 'Failed',
};

const App = {
    currentView: 'posts',
    posts: [],
    jobs: [],
    config: null,
    resources: null,
    selectedSlug: null,
    pollTimer: null,

    // ── Initialization ────────────────────────────────────────────────────────

    async init() {
        this.bindNavigation();
        this.bindSearch();
        this.bindToggles();
        await Promise.all([
            this.loadPosts(),
            this.loadConfig(),
            this.loadJobs(),
            this.loadResources(),
        ]);
        this.startJobPolling();
    },

    // ── Navigation ────────────────────────────────────────────────────────────

    bindNavigation() {
        document.querySelectorAll('.nav-item').forEach(btn => {
            btn.addEventListener('click', () => {
                const view = btn.dataset.view;
                this.switchView(view);
            });
        });
    },

    switchView(view) {
        this.currentView = view;

        document.querySelectorAll('.nav-item').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.view === view);
        });

        document.querySelectorAll('.view').forEach(v => {
            v.classList.toggle('active', v.id === `view-${view}`);
        });

        switch (view) {
            case 'posts': this.loadPosts(); break;
            case 'jobs': this.loadJobs(); break;
            case 'videos': this.loadVideos(); break;
            case 'settings': this.loadConfig(); break;
        }
    },

    // ── Search ────────────────────────────────────────────────────────────────

    bindSearch() {
        const input = document.getElementById('post-search');
        input.addEventListener('input', () => {
            this.filterPosts(input.value.trim().toLowerCase());
        });
    },

    filterPosts(query) {
        const cards = document.querySelectorAll('#posts-list .card');
        cards.forEach(card => {
            const title = card.dataset.title || '';
            const slug = card.dataset.slug || '';
            const match = !query || title.includes(query) || slug.includes(query);
            card.style.display = match ? '' : 'none';
        });
    },

    // ── Toggle bindings ───────────────────────────────────────────────────────

    bindToggles() {
        const concatToggle = document.getElementById('modal-concat');
        const ckToggle = document.getElementById('modal-chromakey');

        concatToggle.addEventListener('change', () => {
            document.getElementById('concat-options').classList.toggle('hidden', !concatToggle.checked);
        });

        ckToggle.addEventListener('change', () => {
            document.getElementById('chromakey-options').classList.toggle('hidden', !ckToggle.checked);
        });
    },

    // ── Posts ──────────────────────────────────────────────────────────────────

    async loadPosts() {
        const container = document.getElementById('posts-list');
        container.innerHTML = '<div class="loading-state">Loading posts...</div>';

        try {
            const res = await fetch('/api/posts');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.posts = data.posts || [];
            this.renderPosts();
        } catch (err) {
            container.innerHTML = `<div class="error-state">Failed to load posts: ${err.message}</div>`;
        }
    },

    renderPosts() {
        const container = document.getElementById('posts-list');

        if (this.posts.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="48" height="48">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                        <polyline points="14 2 14 8 20 8"></polyline>
                    </svg>
                    <p>No posts found. Check that BLOG_POSTS_DIR is configured correctly.</p>
                </div>`;
            return;
        }

        container.innerHTML = this.posts.map(post => `
            <div class="card" data-slug="${post.slug}" data-title="${post.title.toLowerCase()}"
                 onclick="App.openGenerateModal('${post.slug}')">
                <div class="card-header">
                    <div class="card-title">${this.escapeHtml(post.title)}</div>
                    <div class="card-date">${post.date}</div>
                </div>
                <div class="card-excerpt">${this.escapeHtml(post.excerpt)}</div>
                <div class="card-footer">
                    <span class="card-badge ${post.has_video ? 'has-video' : 'no-video'}">
                        ${post.has_video
                            ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12"><polyline points="20 6 9 17 4 12"></polyline></svg> Video ready'
                            : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg> No video'
                        }
                    </span>
                    <button class="btn btn-primary btn-small" onclick="event.stopPropagation(); App.openGenerateModal('${post.slug}')">
                        Generate
                    </button>
                </div>
            </div>
        `).join('');
    },

    // ── Resources ─────────────────────────────────────────────────────────────

    async loadResources() {
        try {
            const res = await fetch('/api/resources');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            this.resources = await res.json();
        } catch (err) {
            console.warn('Failed to load resources:', err.message);
            this.resources = { intros: [], outros: [], backgrounds: [] };
        }
    },

    // ── Generate Modal ────────────────────────────────────────────────────────

    openGenerateModal(slug) {
        this.selectedSlug = slug;
        const post = this.posts.find(p => p.slug === slug);
        if (!post) return;

        document.getElementById('modal-post-title').textContent = `${post.title} (${post.date})`;
        document.getElementById('modal-text').value = '';

        // Populate avatar dropdown from config.
        const avatarSelect = document.getElementById('modal-avatar');
        if (this.config && this.config.avatars && this.config.avatars.length > 0) {
            avatarSelect.innerHTML = this.config.avatars.map(a =>
                `<option value="${a}" ${a === this.config.default_avatar ? 'selected' : ''}>${a}</option>`
            ).join('');
        } else {
            avatarSelect.innerHTML = '<option value="eve" selected>eve</option>';
        }

        // Set concat toggle from server default.
        const concatToggle = document.getElementById('modal-concat');
        concatToggle.checked = this.config ? this.config.concat_enabled !== false : true;
        document.getElementById('concat-options').classList.toggle('hidden', !concatToggle.checked);

        // Set chroma-key toggle from server default.
        const ckToggle = document.getElementById('modal-chromakey');
        ckToggle.checked = this.config ? !!this.config.chroma_key_enabled : false;
        document.getElementById('chromakey-options').classList.toggle('hidden', !ckToggle.checked);

        // Populate intro/outro/background dropdowns from resources.
        this.populateResourceDropdown('modal-intro', this.resources?.intros || [], 'Server default');
        this.populateResourceDropdown('modal-outro', this.resources?.outros || [], 'Server default');
        this.populateResourceDropdown('modal-bg', this.resources?.backgrounds || [], 'Server default');

        document.getElementById('generate-modal').classList.remove('hidden');
    },

    populateResourceDropdown(selectId, items, defaultLabel) {
        const select = document.getElementById(selectId);
        select.innerHTML = `<option value="">${defaultLabel}</option>` +
            items.map(f => `<option value="${f}">${f}</option>`).join('');
    },

    closeModal() {
        document.getElementById('generate-modal').classList.add('hidden');
        this.selectedSlug = null;
    },

    async submitGenerate() {
        const slug = this.selectedSlug;
        if (!slug) return;

        const avatarID = document.getElementById('modal-avatar').value;
        const text = document.getElementById('modal-text').value.trim();
        const concatEnabled = document.getElementById('modal-concat').checked;
        const chromaKeyEnabled = document.getElementById('modal-chromakey').checked;
        const concatIntro = document.getElementById('modal-intro').value;
        const concatOutro = document.getElementById('modal-outro').value;
        const chromaKeyBG = document.getElementById('modal-bg').value;
        const submitBtn = document.getElementById('modal-submit');

        submitBtn.disabled = true;
        submitBtn.textContent = 'Submitting...';

        try {
            const body = {
                slug,
                avatar_id: avatarID,
                concat_enabled: concatEnabled,
                chroma_key_enabled: chromaKeyEnabled,
            };
            if (text) body.text = text;
            if (concatIntro) body.concat_intro = concatIntro;
            if (concatOutro) body.concat_outro = concatOutro;
            if (chromaKeyBG) body.chroma_key_bg = chromaKeyBG;

            const res = await fetch('/api/generate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });

            if (!res.ok) {
                const err = await res.json();
                throw new Error(err.error || `HTTP ${res.status}`);
            }

            const data = await res.json();
            this.toast(`Pipeline started (Job: ${data.job_id.slice(0, 8)}...)`, 'success');
            this.closeModal();
            this.switchView('jobs');
            this.loadJobs();
        } catch (err) {
            this.toast(`Failed: ${err.message}`, 'error');
        } finally {
            submitBtn.disabled = false;
            submitBtn.innerHTML = `
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>
                Generate Video`;
        }
    },

    // ── Jobs ──────────────────────────────────────────────────────────────────

    async loadJobs() {
        try {
            const res = await fetch('/api/jobs');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.jobs = data.jobs || [];
            this.renderJobs();
            this.updateJobBadge();
        } catch (err) {
            console.error('Failed to load jobs:', err);
        }
    },

    renderJobs() {
        const container = document.getElementById('jobs-list');

        if (this.jobs.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="48" height="48">
                        <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"></polyline>
                    </svg>
                    <p>No jobs yet. Generate a video from the Posts tab.</p>
                </div>`;
            return;
        }

        // Sort newest first.
        const sorted = [...this.jobs].sort((a, b) =>
            new Date(b.created_at) - new Date(a.created_at)
        );

        container.innerHTML = sorted.map(job => this.renderJobCard(job)).join('');
    },

    renderJobCard(job) {
        const phase = job.phase || 'queued';
        const isActive = !['completed', 'failed'].includes(phase);
        const isFailed = phase === 'failed';
        const isComplete = phase === 'completed';
        const progressPct = Math.round(job.progress * 100);

        const phaseClass = isComplete ? 'phase-completed' : isFailed ? 'phase-failed' : isActive ? 'phase-active' : 'phase-queued';

        // Build pipeline phase steps visualization.
        const pipelineSteps = PHASES.map(p => {
            const phaseIdx = PHASES.indexOf(phase);
            const stepIdx = PHASES.indexOf(p);
            let cls = '';
            if (isFailed && p === phase) cls = 'failed';
            else if (isComplete || stepIdx < phaseIdx) cls = 'done';
            else if (p === phase) cls = 'current';
            return `<span class="phase-step ${cls}">${p.slice(0, 5)}</span>`;
        }).join('');

        // Job option tags.
        const optionTags = [];
        optionTags.push(`<span class="job-option-tag">${this.escapeHtml(job.avatar_id || 'eve')}</span>`);
        if (job.concat_enabled) {
            optionTags.push('<span class="job-option-tag enabled">concat</span>');
            if (job.concat_intro) optionTags.push(`<span class="job-option-tag">${this.escapeHtml(job.concat_intro)}</span>`);
        }
        if (job.chroma_key_enabled) {
            optionTags.push('<span class="job-option-tag enabled">chroma-key</span>');
            if (job.chroma_key_bg) optionTags.push(`<span class="job-option-tag">${this.escapeHtml(job.chroma_key_bg)}</span>`);
        }

        return `
            <div class="job-card ${phaseClass}">
                <div class="job-header">
                    <div class="job-title">${this.escapeHtml(job.slug || job.id)}</div>
                    <span class="job-phase ${phaseClass}">${PHASE_LABELS[phase] || phase}</span>
                </div>
                <div class="job-meta">
                    <span>ID: ${job.id.slice(0, 12)}...</span>
                    <span>${this.timeAgo(job.updated_at)}</span>
                </div>
                <div class="job-options">${optionTags.join('')}</div>
                <div class="pipeline-progress">
                    <div class="pipeline-bar">
                        <div class="pipeline-fill ${isActive ? 'active' : ''} ${isComplete ? 'completed' : ''} ${isFailed ? 'failed' : ''}"
                             style="width: ${progressPct}%"></div>
                    </div>
                    <div class="pipeline-phases">${pipelineSteps}</div>
                </div>
                ${isFailed && job.error ? `<div class="job-error">${this.escapeHtml(job.error)}</div>` : ''}
                ${isComplete && job.video_file ? `
                    <div class="job-actions">
                        <button class="btn btn-primary btn-small" onclick="App.playVideo('${job.video_file}')">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>
                            Play
                        </button>
                        <a href="/api/videos/${job.video_file}" download class="btn btn-secondary btn-small">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
                            Download
                        </a>
                    </div>
                ` : ''}
            </div>
        `;
    },

    updateJobBadge() {
        const active = this.jobs.filter(j =>
            !['completed', 'failed'].includes(j.phase)
        ).length;

        const badge = document.getElementById('active-jobs-badge');
        if (active > 0) {
            badge.textContent = active;
            badge.classList.remove('hidden');
        } else {
            badge.classList.add('hidden');
        }
    },

    startJobPolling() {
        if (this.pollTimer) clearInterval(this.pollTimer);
        this.pollTimer = setInterval(() => {
            const hasActive = this.jobs.some(j =>
                !['completed', 'failed'].includes(j.phase)
            );
            if (hasActive || this.currentView === 'jobs') {
                this.loadJobs();
            }
        }, 3000);
    },

    // ── Videos ────────────────────────────────────────────────────────────────

    async loadVideos() {
        const container = document.getElementById('videos-list');
        container.innerHTML = '<div class="loading-state">Loading videos...</div>';

        try {
            const res = await fetch('/api/videos');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.renderVideos(data.videos || []);
        } catch (err) {
            container.innerHTML = `<div class="error-state">Failed to load videos: ${err.message}</div>`;
        }
    },

    renderVideos(videos) {
        const container = document.getElementById('videos-list');

        if (videos.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="48" height="48">
                        <polygon points="5 3 19 12 5 21 5 3"></polygon>
                    </svg>
                    <p>No videos generated yet.</p>
                </div>`;
            return;
        }

        container.innerHTML = videos.map(v => `
            <div class="video-card" onclick="App.playVideo('${v.name}')">
                <div class="video-preview">
                    <video preload="metadata" muted>
                        <source src="/api/videos/${v.name}" type="video/mp4">
                    </video>
                    <div class="play-overlay">
                        <div class="play-icon">
                            <svg viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>
                        </div>
                    </div>
                </div>
                <div class="video-info">
                    <div class="video-name" title="${v.name}">${v.name}</div>
                    <div class="video-meta">
                        <span>${v.size_mb}</span>
                        <span>${v.modified}</span>
                    </div>
                </div>
            </div>
        `).join('');
    },

    playVideo(filename) {
        window.open(`/api/videos/${filename}`, '_blank');
    },

    // ── Config ────────────────────────────────────────────────────────────────

    async loadConfig() {
        try {
            const res = await fetch('/api/config');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            this.config = await res.json();
            this.renderConfig();
            this.updateAPIStatus();
        } catch (err) {
            document.getElementById('settings-content').innerHTML =
                `<div class="error-state">Failed to load configuration: ${err.message}</div>`;
            this.updateAPIStatus();
        }
    },

    renderConfig() {
        if (!this.config) return;

        const container = document.getElementById('settings-content');
        const c = this.config;

        const avatarTags = (c.avatars || []).map(a =>
            `<span class="setting-tag">${a}</span>`
        ).join('');

        container.innerHTML = `
            <div class="setting-card">
                <div class="setting-label">Narrator API</div>
                <div class="setting-value url">${this.escapeHtml(c.narrator_url || 'Not configured')}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Narrator Status</div>
                <div class="setting-value ${c.narrator_online ? 'bool-on' : 'bool-off'}">${c.narrator_online ? 'Online' : 'Offline'}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">MuseTalk API</div>
                <div class="setting-value url">${this.escapeHtml(c.musetalk_url || 'N/A')}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Default Avatar</div>
                <div class="setting-value">${this.escapeHtml(c.default_avatar || 'eve')}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Available Avatars</div>
                <div class="setting-value list">${avatarTags || '<span class="setting-tag">eve</span>'}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Concat (Intro/Outro)</div>
                <div class="setting-value ${c.concat_enabled ? 'bool-on' : 'bool-off'}">${c.concat_enabled ? 'Enabled' : 'Disabled'}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Chroma Key</div>
                <div class="setting-value ${c.chroma_key_enabled ? 'bool-on' : 'bool-off'}">${c.chroma_key_enabled ? 'Enabled' : 'Disabled'}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Posts Directory</div>
                <div class="setting-value">${this.escapeHtml(c.posts_dir || 'Not configured')}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Output Directory</div>
                <div class="setting-value">${this.escapeHtml(c.output_dir || 'Not configured')}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Poll Interval</div>
                <div class="setting-value">${c.poll_interval || 'N/A'}</div>
            </div>
            <div class="setting-card">
                <div class="setting-label">Max Wait Time</div>
                <div class="setting-value">${c.max_wait || 'N/A'}</div>
            </div>
        `;
    },

    updateAPIStatus() {
        const dot = document.querySelector('.status-dot');
        const text = document.querySelector('.status-text');

        if (this.config && this.config.narrator_online) {
            dot.className = 'status-dot online';
            text.textContent = 'Narrator Online';
        } else if (this.config) {
            dot.className = 'status-dot degraded';
            text.textContent = 'Narrator Offline';
        } else {
            dot.className = 'status-dot offline';
            text.textContent = 'API Error';
        }
    },

    // ── Toast Notifications ───────────────────────────────────────────────────

    toast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        const el = document.createElement('div');
        el.className = `toast ${type}`;
        el.innerHTML = `<span class="toast-dot"></span><span>${this.escapeHtml(message)}</span>`;
        container.appendChild(el);

        setTimeout(() => {
            el.style.opacity = '0';
            el.style.transform = 'translateX(100%)';
            el.style.transition = 'all 200ms ease';
            setTimeout(() => el.remove(), 200);
        }, 4000);
    },

    // ── Utilities ─────────────────────────────────────────────────────────────

    escapeHtml(text) {
        if (!text) return '';
        const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
        return String(text).replace(/[&<>"']/g, c => map[c]);
    },

    timeAgo(dateStr) {
        if (!dateStr) return '';
        const now = new Date();
        const date = new Date(dateStr);
        const seconds = Math.floor((now - date) / 1000);

        if (seconds < 60) return 'just now';
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
        if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
        return `${Math.floor(seconds / 86400)}d ago`;
    },
};

// ── Boot ──────────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => App.init());
