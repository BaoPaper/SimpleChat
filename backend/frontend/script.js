// ==================== SimpleChat 前端脚本 ====================

// ---- 全局状态 ----
const state = {
    token: localStorage.getItem('simplechat_token') || '',
    username: '',
    sessions: [],
    currentSessionId: null,
    messages: [],
    models: [],
    defaultModel: '',
    currentModel: '',
    isStreaming: false,
};

// ---- API 客户端 ----
async function api(path, options = {}) {
    const headers = {
        'Content-Type': 'application/json',
        ...options.headers,
    };
    if (state.token) {
        headers['Authorization'] = `Bearer ${state.token}`;
    }
    const res = await fetch(path, { ...options, headers });
    return res;
}

async function apiJSON(path, options = {}) {
    const res = await api(path, options);
    const data = await res.json();
    if (!res.ok) {
        throw new Error(data.error || `请求失败 (${res.status})`);
    }
    return data;
}

// ---- 登录 ----
async function login(username, password) {
    const data = await apiJSON('/api/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
    });
    state.token = data.token;
    state.username = data.username || username || '用户';
    localStorage.setItem('simplechat_token', state.token);
    return data;
}

function logout() {
    state.token = '';
    state.username = '';
    localStorage.removeItem('simplechat_token');
    state.sessions = [];
    state.currentSessionId = null;
    state.messages = [];
    showLogin();
}

async function checkAuth() {
    if (!state.token) return false;
    try {
        const data = await apiJSON('/api/check');
        state.username = data.username || '';
        return true;
    } catch {
        state.token = '';
        localStorage.removeItem('simplechat_token');
        return false;
    }
}

// ---- 会话管理 ----
async function loadSessions() {
    const data = await apiJSON('/api/sessions');
    state.sessions = data.sessions || [];
    renderSessionList();
}

async function createSession(options = {}) {
    const { loadMessages = true } = options;
    const data = await apiJSON('/api/sessions', {
        method: 'POST',
        body: JSON.stringify({ title: '新对话' }),
    });
    state.sessions.unshift(data.session);

    if (loadMessages) {
        switchSession(data.session.id);
    } else {
        state.currentSessionId = data.session.id;
        localStorage.setItem('simplechat_last_session', data.session.id);
        renderSessionList();
    }

    return data.session;
}

async function deleteSession(id) {
    await apiJSON(`/api/sessions/${id}`, { method: 'DELETE' });
    state.sessions = state.sessions.filter(s => s.id !== id);
    if (state.currentSessionId === id) {
        state.currentSessionId = null;
        state.messages = [];
        showGreeting();
    }
    renderSessionList();
}

async function renameSession(id, title) {
    await apiJSON(`/api/sessions/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ title }),
    });
    const s = state.sessions.find(s => s.id === id);
    if (s) s.title = title;
    renderSessionList();
}

async function loadSessionMessages(id) {
    const data = await apiJSON(`/api/sessions/${id}`);
    state.messages = data.messages || [];
    renderMessages();
}

function switchSession(id) {
    state.currentSessionId = id;
    localStorage.setItem('simplechat_last_session', id);
    state.messages = [];
    const msgContainer = document.getElementById('messagesContainer');
    msgContainer.style.display = 'none';
    msgContainer.innerHTML = '';
    document.getElementById('greetingContainer').style.display = 'none';

    loadSessionMessages(id).then(() => {
        scrollToBottom();
    });

    renderSessionList();
}

// ---- 模型 ----
async function loadModels() {
    const data = await apiJSON('/api/models');
    state.models = data.models || [];
    state.defaultModel = data.default_model || (state.models[0]?.id || '');
    state.currentModel = localStorage.getItem('simplechat_model') || state.defaultModel;

    if (!state.models.find(m => m.id === state.currentModel)) {
        state.currentModel = state.defaultModel;
    }

    updateModelDisplay();
    renderModelDropdown();
}

function setModel(modelId) {
    state.currentModel = modelId;
    localStorage.setItem('simplechat_model', modelId);
    updateModelDisplay();
    renderModelDropdown();
    closeAllDropdowns();
}

function updateModelDisplay() {
    const model = state.models.find(m => m.id === state.currentModel);
    document.getElementById('currentModelName').textContent = model ? model.name : state.currentModel;
}

function renderModelDropdown() {
    const dropdown = document.getElementById('modelDropdown');
    dropdown.innerHTML = state.models.map(m => `
        <button class="dropdown-item model-option ${m.id === state.currentModel ? 'active' : ''}"
                data-model-id="${m.id}">
            <span>${escapeHtml(m.name)}</span>
            ${m.id === state.currentModel ? '<span class="check-mark">✓</span>' : ''}
        </button>
    `).join('');
}

// ---- 聊天 ----
async function sendMessage() {
    if (state.isStreaming) return;

    const input = document.getElementById('chatInput');
    const message = input.value.trim();
    if (!message) return;

    if (!state.currentSessionId) {
        await createSession({ loadMessages: false });
    }

    input.value = '';
    input.style.height = 'auto';
    document.getElementById('sendBtn').setAttribute('disabled', 'true');

    addMessageBubble('user', message);
    scrollToBottom();

    const assistantEl = addMessageBubble('assistant', '');
    const contentEl = assistantEl.querySelector('.message-content');
    let fullContent = '';

    state.isStreaming = true;

    try {
        const response = await fetch('/api/chat', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${state.token}`,
            },
            body: JSON.stringify({
                session_id: state.currentSessionId,
                model: state.currentModel,
                message: message,
            }),
        });

        if (!response.ok) {
            const errData = await response.json().catch(() => ({}));
            throw new Error(errData.error || `请求失败 (${response.status})`);
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
                if (!line.startsWith('data: ')) continue;
                try {
                    const event = JSON.parse(line.slice(6));
                    handleSSEEvent(event, contentEl, (c) => { fullContent = c; });
                } catch {
                    // 忽略
                }
            }
        }

        if (buffer.startsWith('data: ')) {
            try {
                const event = JSON.parse(buffer.slice(6));
                handleSSEEvent(event, contentEl, (c) => { fullContent = c; });
            } catch { /* ignore */ }
        }
    } catch (err) {
        contentEl.innerHTML = `<span class="error-text">错误: ${escapeHtml(err.message)}</span>`;
        console.error('Chat error:', err);
    }

    state.isStreaming = false;
    scrollToBottom();
    loadSessions();
}

function handleSSEEvent(event, contentEl, setFullContent) {
    switch (event.type) {
        case 'meta':
            if (event.session_id && !state.currentSessionId) {
                state.currentSessionId = event.session_id;
            }
            break;
        case 'content':
            const currentText = contentEl.getAttribute('data-raw') || '';
            const newText = currentText + (event.content || '');
            contentEl.setAttribute('data-raw', newText);
            contentEl.innerHTML = renderMarkdown(newText);
            highlightCodeBlocks(contentEl);
            setFullContent(newText);
            scrollToBottom();
            break;
        case 'done':
            setFullContent(contentEl.getAttribute('data-raw') || '');
            break;
        case 'error':
            contentEl.innerHTML = `<span class="error-text">错误: ${escapeHtml(event.error || event.content || '未知错误')}</span>`;
            break;
    }
}

function addMessageBubble(role, content) {
    const container = document.getElementById('messagesContainer');
    document.getElementById('greetingContainer').style.display = 'none';
    container.style.display = 'flex';

    const div = document.createElement('div');
    div.className = `message-bubble ${role}`;

    const avatar = document.createElement('div');
    avatar.className = 'message-avatar';
    avatar.textContent = role === 'user' ? '👤' : '🤖';

    const body = document.createElement('div');
    body.className = 'message-body';

    const roleLabel = document.createElement('div');
    roleLabel.className = 'message-role';
    roleLabel.textContent = role === 'user' ? (state.username || '你') : 'AI';

    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';

    if (role === 'user') {
        contentDiv.textContent = content;
    } else {
        contentDiv.setAttribute('data-raw', content);
        contentDiv.innerHTML = renderMarkdown(content);
        highlightCodeBlocks(contentDiv);
    }

    body.appendChild(roleLabel);
    body.appendChild(contentDiv);
    div.appendChild(avatar);
    div.appendChild(body);
    container.appendChild(div);

    return div;
}

function renderMessages() {
    const container = document.getElementById('messagesContainer');
    const greeting = document.getElementById('greetingContainer');

    if (state.messages.length === 0) {
        container.style.display = 'none';
        container.innerHTML = '';
        greeting.style.display = 'flex';
        return;
    }

    greeting.style.display = 'none';
    container.style.display = 'flex';
    container.innerHTML = '';

    for (const msg of state.messages) {
        const div = document.createElement('div');
        div.className = `message-bubble ${msg.role}`;

        const avatar = document.createElement('div');
        avatar.className = 'message-avatar';
        avatar.textContent = msg.role === 'user' ? '👤' : '🤖';

        const body = document.createElement('div');
        body.className = 'message-body';

        const roleLabel = document.createElement('div');
        roleLabel.className = 'message-role';
        roleLabel.textContent = msg.role === 'user' ? (state.username || '你') : 'AI';

        const contentDiv = document.createElement('div');
        contentDiv.className = 'message-content';
        contentDiv.innerHTML = renderMarkdown(msg.content);
        highlightCodeBlocks(contentDiv);

        body.appendChild(roleLabel);
        body.appendChild(contentDiv);
        div.appendChild(avatar);
        div.appendChild(body);
        container.appendChild(div);
    }

    scrollToBottom();
}

// ---- Markdown 渲染 ----
function renderMarkdown(text) {
    if (!text) return '';
    if (typeof marked !== 'undefined') {
        marked.setOptions({ breaks: true, gfm: true });
        return marked.parse(text);
    }
    return escapeHtml(text).replace(/\n/g, '<br>');
}

function highlightCodeBlocks(container) {
    if (typeof hljs === 'undefined') return;
    container.querySelectorAll('pre code').forEach((block) => {
        hljs.highlightElement(block);
    });
}

// ---- UI 渲染 ----
function renderSessionList() {
    const list = document.getElementById('chatList');
    const noSessions = document.getElementById('noSessions');

    if (state.sessions.length === 0) {
        list.innerHTML = '';
        noSessions.style.display = 'block';
        return;
    }

    noSessions.style.display = 'none';

    list.innerHTML = state.sessions.map(s => `
        <li class="chat-item ${s.id === state.currentSessionId ? 'active' : ''}"
            data-session-id="${s.id}">
            <span class="chat-title" title="${escapeHtml(s.title)}">${escapeHtml(s.title)}</span>
            <div class="chat-actions">
                <button class="icon-btn more-btn" data-action="more">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="1"></circle><circle cx="19" cy="12" r="1"></circle><circle cx="5" cy="12" r="1"></circle></svg>
                </button>
                <div class="dropdown-menu chat-item-dropdown">
                    <button class="dropdown-item" data-action="rename">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>
                        重命名
                    </button>
                    <button class="dropdown-item text-danger" data-action="delete">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg>
                        删除
                    </button>
                </div>
            </div>
        </li>
    `).join('');
}

function showGreeting() {
    document.getElementById('messagesContainer').style.display = 'none';
    document.getElementById('greetingContainer').style.display = 'flex';
    updateGreeting();
}

function updateGreeting() {
    const hour = new Date().getHours();
    let greeting;
    if (hour < 6) greeting = '夜深了';
    else if (hour < 12) greeting = '上午好';
    else if (hour < 14) greeting = '中午好';
    else if (hour < 18) greeting = '下午好';
    else greeting = '晚上好';
    document.getElementById('greetingText').textContent = `${greeting}，随时开始。`;
}

function showLogin() {
    document.getElementById('loginOverlay').style.display = 'flex';
    document.getElementById('appContainer').style.display = 'none';
    document.getElementById('usernameInput').value = '';
    document.getElementById('passwordInput').value = '';
    document.getElementById('loginError').style.display = 'none';
}

function showApp() {
    document.getElementById('loginOverlay').style.display = 'none';
    document.getElementById('appContainer').style.display = 'flex';
    document.getElementById('displayName').textContent = state.username || '用户';
    document.getElementById('chatInput').removeAttribute('disabled');
}

// ---- 辅助函数 ----
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function scrollToBottom() {
    requestAnimationFrame(() => {
        const area = document.getElementById('chatArea');
        area.scrollTop = area.scrollHeight;
    });
}

function closeAllDropdowns() {
    document.querySelectorAll('.dropdown-menu.show').forEach(menu => {
        menu.classList.remove('show');
        const actions = menu.closest('.chat-actions');
        if (actions) actions.classList.remove('show-menu');
    });
}

// ==================== 事件处理 ====================
document.addEventListener('DOMContentLoaded', () => {
    // === DOM 元素 ===
    const loginOverlay = document.getElementById('loginOverlay');
    const usernameInput = document.getElementById('usernameInput');
    const passwordInput = document.getElementById('passwordInput');
    const loginBtn = document.getElementById('loginBtn');
    const loginError = document.getElementById('loginError');

    const appContainer = document.getElementById('appContainer');
    const chatInput = document.getElementById('chatInput');
    const sendBtn = document.getElementById('sendBtn');
    const newChatBtn = document.getElementById('newChatBtn');
    const chatList = document.getElementById('chatList');
    const modelSelectorBtn = document.getElementById('modelSelectorBtn');
    const modelDropdown = document.getElementById('modelDropdown');
    const userProfileBtn = document.getElementById('userProfileBtn');
    const userDropdown = document.getElementById('userDropdown');
    const themeToggleBtn = document.getElementById('themeToggleBtn');
    const logoutBtn = document.getElementById('logoutBtn');
    const sidebarToggle = document.getElementById('sidebarToggle');
    const sidebarExpandBtn = document.getElementById('sidebarExpandBtn');
    const sidebar = document.getElementById('sidebar');

    // === 登录 ===
    usernameInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') passwordInput.focus();
    });
    passwordInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') performLogin();
    });

    loginBtn.addEventListener('click', performLogin);

    async function performLogin() {
        const username = usernameInput.value.trim();
        const password = passwordInput.value.trim();
        if (!username || !password) {
            loginError.textContent = '请输入用户名和密码';
            loginError.style.display = 'block';
            return;
        }

        loginBtn.disabled = true;
        loginBtn.textContent = '登录中...';
        loginError.style.display = 'none';

        try {
            await login(username, password);
            await initApp();
        } catch (err) {
            loginError.textContent = err.message || '登录失败';
            loginError.style.display = 'block';
        } finally {
            loginBtn.disabled = false;
            loginBtn.textContent = '登 录';
        }
    }

    logoutBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        logout();
    });

    // === 初始化应用 ===
    async function initApp() {
        showApp();
        try {
            await Promise.all([loadModels(), loadSessions()]);
            updateGreeting();
            chatInput.focus();

            const lastSessionId = localStorage.getItem('simplechat_last_session');
            if (lastSessionId && state.sessions.find(s => s.id === lastSessionId)) {
                switchSession(lastSessionId);
            }
        } catch (err) {
            console.error('初始化失败:', err);
        }
    }

    // 检查是否已登录
    (async () => {
        if (state.token) {
            const ok = await checkAuth();
            if (ok) {
                await initApp();
                return;
            }
        }
        showLogin();
        document.getElementById('usernameInput').focus();
    })();

    // === 新对话 ===
    newChatBtn.addEventListener('click', async () => {
        if (state.isStreaming) return;
        try {
            await createSession();
            chatInput.focus();
        } catch (err) {
            console.error('创建会话失败:', err);
        }
    });

    // === 侧边栏折叠 ===
    const isMobile = () => window.innerWidth <= 768;

    function collapseSidebar() {
        sidebar.classList.add('collapsed');
        sidebarExpandBtn.style.display = 'flex';
    }

    function expandSidebar() {
        sidebar.classList.remove('collapsed');
        sidebarExpandBtn.style.display = 'none';
    }

    // 根据屏幕尺寸自动处理侧边栏状态
    function autoSidebarState() {
        if (isMobile()) {
            collapseSidebar();
        } else {
            expandSidebar();
        }
    }

    sidebarToggle.addEventListener('click', collapseSidebar);
    sidebarExpandBtn.addEventListener('click', expandSidebar);

    // 窗口大小变化时自动调整
    let lastWasMobile = isMobile();
    window.addEventListener('resize', () => {
        const nowMobile = isMobile();
        if (nowMobile !== lastWasMobile) {
            lastWasMobile = nowMobile;
            autoSidebarState();
        }
    });

    // 初始化时根据屏幕尺寸设置侧边栏状态
    if (isMobile()) {
        collapseSidebar();
    }

    // === 侧边栏会话点击 ===
    chatList.addEventListener('click', async (e) => {
        const chatItem = e.target.closest('.chat-item');
        if (!chatItem) return;

        const sessionId = chatItem.dataset.sessionId;
        const action = e.target.closest('[data-action]')?.dataset?.action;

        if (action === 'more') {
            e.stopPropagation();
            const actions = chatItem.querySelector('.chat-actions');
            const dropdown = actions.querySelector('.chat-item-dropdown');
            const isShowing = dropdown.classList.contains('show');
            closeAllDropdowns();
            if (!isShowing) {
                dropdown.classList.add('show');
                actions.classList.add('show-menu');
            }
            return;
        }

        if (action === 'rename') {
            e.stopPropagation();
            closeAllDropdowns();
            const s = state.sessions.find(s => s.id === sessionId);
            const newTitle = prompt('请输入新名称:', s?.title || '');
            if (newTitle && newTitle.trim()) {
                await renameSession(sessionId, newTitle.trim());
            }
            return;
        }

        if (action === 'delete') {
            e.stopPropagation();
            closeAllDropdowns();
            if (confirm('确定要删除这个对话吗？')) {
                await deleteSession(sessionId);
            }
            return;
        }

        // 切换会话
        if (state.isStreaming) return;
        switchSession(sessionId);
        localStorage.setItem('simplechat_last_session', sessionId);
    });

    // === 模型选择 ===
    modelSelectorBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isShowing = modelDropdown.classList.contains('show');
        closeAllDropdowns();
        if (!isShowing) {
            modelDropdown.classList.add('show');
        }
    });

    modelDropdown.addEventListener('click', (e) => {
        const option = e.target.closest('.model-option');
        if (!option) return;
        e.stopPropagation();
        setModel(option.dataset.modelId);
    });

    // === 用户菜单 ===
    userProfileBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isShowing = userDropdown.classList.contains('show');
        closeAllDropdowns();
        if (!isShowing) userDropdown.classList.add('show');
    });

    // === 输入框 ===
    chatInput.addEventListener('input', function() {
        this.style.height = 'auto';
        this.style.height = (this.scrollHeight) + 'px';

        if (this.value.trim().length > 0 && !state.isStreaming) {
            sendBtn.removeAttribute('disabled');
        } else {
            sendBtn.setAttribute('disabled', 'true');
        }
    });

    chatInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
            if (e.ctrlKey || e.metaKey) {
                e.preventDefault();
                const start = this.selectionStart;
                const end = this.selectionEnd;
                this.value = this.value.substring(0, start) + "\n" + this.value.substring(end);
                this.selectionStart = this.selectionEnd = start + 1;
                this.dispatchEvent(new Event('input'));
            } else if (!e.shiftKey) {
                e.preventDefault();
                if (this.value.trim().length > 0 && !state.isStreaming) {
                    sendMessage();
                }
            }
        }
    });

    sendBtn.addEventListener('click', () => {
        if (chatInput.value.trim().length > 0 && !state.isStreaming) {
            sendMessage();
        }
    });

    // === 全局点击关闭下拉菜单 ===
    document.addEventListener('click', (e) => {
        // 不关闭正在处理的 modelDropdown 点击
        if (e.target.closest('#modelDropdown')) return;
        closeAllDropdowns();
    });

    // === 暗色模式 ===
    const currentTheme = localStorage.getItem('theme');
    if (currentTheme === 'dark') {
        document.body.classList.add('dark-mode');
        updateThemeButtonText(true);
    }

    themeToggleBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isDark = document.body.classList.toggle('dark-mode');
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
        updateThemeButtonText(isDark);
    });

    function updateThemeButtonText(isDark) {
        const span = themeToggleBtn.querySelector('span');
        const svg = themeToggleBtn.querySelector('svg');
        if (isDark) {
            span.textContent = '切换至浅色模式';
            svg.innerHTML = '<circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>';
        } else {
            span.textContent = '切换至暗色模式';
            svg.innerHTML = '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>';
        }
    }
});
