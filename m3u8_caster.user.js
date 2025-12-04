// ==UserScript==
// @name         M3U8 Caster
// @namespace    http://tampermonkey.net/
// @version      0.1
// @description  Detect m3u8 videos and cast them to local DLNA service
// @author       You
// @match        *://*/*
// @grant        GM_xmlhttpRequest
// @connect      172.16.1.5
// ==/UserScript==

(function() {
    'use strict';

    let detectedUrls = [];
    let currentIndex = 0;
    const containerId = 'm3u8-caster-container';
    const CAST_API_URL = 'https://172.16.1.5/api/cast';
    const DEVICES_API_URL = 'https://172.16.1.5/api/devices';

    // UI Styles
    function addStyle(css) {
        const style = document.createElement('style');
        style.textContent = css;
        document.head.appendChild(style);
    }

    addStyle(`
        #${containerId} {
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: rgba(0, 0, 0, 0.8);
            color: white;
            padding: 15px;
            border-radius: 8px;
            z-index: 999999;
            display: none;
            font-family: sans-serif;
            max-width: 400px;
            word-break: break-all;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        #${containerId} .url {
            font-size: 12px;
            margin-bottom: 10px;
            color: #ddd;
        }
        #${containerId} select {
            flex-grow: 1;
            padding: 8px;
            background: #333;
            color: white;
            border: 1px solid #555;
            border-radius: 4px;
            margin-right: 5px;
        }
        #${containerId} button {
            background: #4CAF50;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
            margin-right: 5px;
        }
        #${containerId} button:hover {
            background: #45a049;
        }
        #${containerId} button.copy-btn {
            background: #2196F3;
        }
        #${containerId} button.copy-btn:hover {
            background: #0b7dda;
        }
        #${containerId} .close {
            position: absolute;
            top: 5px;
            right: 5px;
            cursor: pointer;
            color: #aaa;
            font-size: 16px;
        }
        @media (max-width: 600px) {
            #${containerId} {
                left: 10px;
                right: 10px;
                bottom: 10px;
                max-width: none;
                padding: 20px;
            }
            #${containerId} button {
                padding: 12px 20px;
                font-size: 16px;
                margin-bottom: 5px;
            }
            #${containerId} select {
                padding: 12px;
                font-size: 16px;
                height: 45px;
            }
            #${containerId} #refresh-btn {
                padding: 0 15px !important;
                height: 45px !important;
                font-size: 18px !important;
            }
            #${containerId} .url-container button {
                padding: 8px 15px !important;
                font-size: 16px !important;
            }
            #${containerId} .close {
                padding: 10px;
                font-size: 24px;
                top: 0;
                right: 0;
            }
        }
    `);

    // Create UI
    function createUI() {
        if (document.getElementById(containerId)) return;

        const container = document.createElement('div');
        container.id = containerId;
        container.innerHTML = `
            <div class="close">&times;</div>
            <div class="close">&times;</div>
            <div class="url-container" style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px;">
                <button id="prev-btn" style="background: #555; padding: 4px 8px; font-size: 12px; display: none;">&lt;</button>
                <div class="url" style="margin: 0 5px; flex-grow: 1; text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"></div>
                <button id="next-btn" style="background: #555; padding: 4px 8px; font-size: 12px; display: none;">&gt;</button>
            </div>
            <div style="display: flex; align-items: center; margin-bottom: 10px;">
                <select id="device-list">
                    <option value="">Loading devices...</option>
                </select>
                <button id="refresh-btn" style="background: #2196F3; font-size: 12px; padding: 8px; height: 34px;">&#x21bb;</button>
            </div>
            <button id="copy-btn" class="copy-btn">Copy</button>
            <button id="cast-btn">Cast to DLNA</button>
        `;
        document.body.appendChild(container);

        // Fetch devices immediately
        fetchDevices();

        container.querySelector('.close').onclick = () => {
            container.style.display = 'none';
        };

        container.querySelector('#copy-btn').onclick = copyLink;
        container.querySelector('#cast-btn').onclick = castVideo;
        container.querySelector('#refresh-btn').onclick = fetchDevices;
        container.querySelector('#prev-btn').onclick = prevVideo;
        container.querySelector('#next-btn').onclick = nextVideo;
    }

    // Fetch Devices
    function fetchDevices() {
        const container = document.getElementById(containerId);
        const refreshBtn = container ? container.querySelector('#refresh-btn') : null;
        const originalBtnText = refreshBtn ? refreshBtn.innerHTML : '&#x21bb;';

        if (refreshBtn) {
            refreshBtn.disabled = true;
            refreshBtn.innerHTML = '...';
        }

        GM_xmlhttpRequest({
            method: "GET",
            url: DEVICES_API_URL,
            onload: function(response) {
                if (refreshBtn) {
                    refreshBtn.disabled = false;
                    refreshBtn.innerHTML = originalBtnText;
                }

                const select = container ? container.querySelector('#device-list') : null;
                if (!select) return;

                if (response.status === 200) {
                    try {
                        const devices = JSON.parse(response.responseText);
                        select.innerHTML = '';
                        
                        if (devices.length === 0) {
                            const option = document.createElement('option');
                            option.text = "No devices found";
                            select.add(option);
                            return;
                        }

                        devices.forEach(device => {
                            const option = document.createElement('option');
                            option.value = device.usn;
                            option.text = device.friendly_name;
                            select.add(option);
                        });
                    } catch (e) {
                        console.error("Error parsing devices:", e);
                        select.innerHTML = '<option>Error parsing devices</option>';
                    }
                } else {
                    select.innerHTML = '<option>Error fetching devices</option>';
                }
            },
            onerror: function(err) {
                if (refreshBtn) {
                    refreshBtn.disabled = false;
                    refreshBtn.innerHTML = originalBtnText;
                }
                console.error("Device fetch error:", err);
                const select = container ? container.querySelector('#device-list') : null;
                if (select) select.innerHTML = '<option>Connection error</option>';
            }
        });
    }

    // Navigation functions
    function prevVideo() {
        if (currentIndex > 0) {
            currentIndex--;
            updateUI();
        }
    }

    function nextVideo() {
        if (currentIndex < detectedUrls.length - 1) {
            currentIndex++;
            updateUI();
        }
    }

    function updateUI() {
        const container = document.getElementById(containerId);
        if (!container || detectedUrls.length === 0) return;

        const item = detectedUrls[currentIndex];
        const url = item.url;
        const displayUrl = url.split('?')[0];
        const typeLabel = item.type === 'video' ? '[Video] ' : '';
        
        const urlDiv = container.querySelector('.url');
        urlDiv.textContent = `[${currentIndex + 1}/${detectedUrls.length}] ${typeLabel}${displayUrl}`;
        urlDiv.title = url;
        urlDiv.style.color = item.type === 'video' ? '#4CAF50' : '#ddd'; // Green for video, default for others

        const prevBtn = container.querySelector('#prev-btn');
        const nextBtn = container.querySelector('#next-btn');

        if (detectedUrls.length > 1) {
            prevBtn.style.display = 'block';
            nextBtn.style.display = 'block';
            prevBtn.disabled = currentIndex === 0;
            nextBtn.disabled = currentIndex === detectedUrls.length - 1;
            prevBtn.style.opacity = prevBtn.disabled ? '0.5' : '1';
            nextBtn.style.opacity = nextBtn.disabled ? '0.5' : '1';
        } else {
            prevBtn.style.display = 'none';
            nextBtn.style.display = 'none';
        }
    }

    // Show Error function
    function showError(msg) {
        const container = document.getElementById(containerId);
        const urlDiv = container.querySelector('.url');
        const originalContent = urlDiv.getAttribute('data-original-content') || urlDiv.innerHTML;
        
        // Save original content if not already saved
        if (!urlDiv.getAttribute('data-original-content')) {
            urlDiv.setAttribute('data-original-content', originalContent);
        }

        urlDiv.innerHTML = `<span style="color: #ff6b6b">Error: ${msg}</span> <span class="error-close" style="cursor:pointer; color: #aaa; margin-left: 5px;">[x]</span>`;
        
        urlDiv.querySelector('.error-close').onclick = () => {
            urlDiv.innerHTML = originalContent;
        };
    }

    // Copy function
    function copyLink() {
        if (detectedUrls.length === 0) return;
        const detectedUrl = detectedUrls[currentIndex].url;
        const btn = document.getElementById('copy-btn');
        
        // Check if in result state (based on custom attribute)
        if (btn.getAttribute('data-state') === 'result') {
            btn.textContent = 'Copy';
            btn.style.background = ''; // Revert to CSS default
            btn.removeAttribute('data-state');
            // Continue to re-execute copy
        }

        const setSuccess = () => {
            btn.textContent = 'Copied!';
            btn.style.background = '#4CAF50'; // Green
            btn.setAttribute('data-state', 'result');
        };

        const setFailure = (msg) => {
            btn.textContent = 'Failed';
            btn.style.background = '#F44336'; // Red
            btn.setAttribute('data-state', 'result');
            showError(msg || 'Copy failed');
        };

        navigator.clipboard.writeText(detectedUrl).then(() => {
            setSuccess();
        }).catch(err => {
            console.error('Failed to copy: ', err);
            setFailure('Copy failed: ' + err);
        });
    }

    // Show UI with URL
    function showUI(url, type) {
        if (!url) return;
        
        // Check if URL already exists
        const existingIndex = detectedUrls.findIndex(u => u.url === url);
        
        if (existingIndex === -1) {
            detectedUrls.push({ url: url, type: type });
            const newIndex = detectedUrls.length - 1;
            
            // Default behavior: always switch to the latest detected URL
            currentIndex = newIndex;
        } else {
            // If already exists, maybe update type if it was network and now found in video tag?
            if (type === 'video' && detectedUrls[existingIndex].type !== 'video') {
                detectedUrls[existingIndex].type = 'video';
                // Update UI if we are currently viewing this item
                if (currentIndex === existingIndex) {
                    updateUI();
                }
            }
        }
        
        createUI();
        const container = document.getElementById(containerId);
        container.style.display = 'block';
        updateUI();
    }

    // Cast function
    function castVideo() {
        if (detectedUrls.length === 0) return;
        const detectedUrl = detectedUrls[currentIndex].url;

        const btn = document.getElementById('cast-btn');
        
        // Check if in result state
        if (btn.getAttribute('data-state') === 'result') {
            btn.textContent = 'Cast to DLNA';
            btn.style.background = ''; // Revert to CSS default
            btn.disabled = false;
            btn.removeAttribute('data-state');
            // Continue to re-execute cast
        }

        btn.textContent = 'Casting...';
        btn.disabled = true;

        const container = document.getElementById(containerId);
        const deviceSelect = container ? container.querySelector('#device-list') : null;
        const selectedUSN = deviceSelect ? deviceSelect.value : '';

        GM_xmlhttpRequest({
            method: "POST",
            url: CAST_API_URL,
            headers: {
                "Content-Type": "application/json"
            },
            data: JSON.stringify({
                url: detectedUrl,
                usn: selectedUSN,
                title: document.title || "Unknown Title"
            }),
            onload: function(response) {
                console.log("Cast response:", response.responseText);
                btn.disabled = false; // Enable to allow click-to-restore
                btn.setAttribute('data-state', 'result');
                
                if (response.status === 200) {
                    btn.textContent = 'Casting Started!';
                    btn.style.background = '#2E7D32'; // Dark Green
                } else {
                    btn.textContent = 'Failed';
                    btn.style.background = '#F44336'; // Red
                    showError('Status ' + response.status + ': ' + response.responseText);
                }
            },
            onerror: function(err) {
                console.error("Cast error:", err);
                btn.disabled = false;
                btn.setAttribute('data-state', 'result');
                btn.textContent = 'Error';
                btn.style.background = '#F44336'; // Red
                showError('Connection error');
            }
        });
    }

    // Intercept Fetch
    const originalFetch = window.fetch;
    window.fetch = async function(...args) {
        const [resource, config] = args;
        if (typeof resource === 'string' && resource.includes('.m3u8')) {
            console.log('M3U8 detected via fetch:', resource);
            showUI(resource, 'network');
        }
        return originalFetch.apply(this, args);
    };

    // Intercept XHR
    const originalOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(method, url) {
        if (typeof url === 'string' && url.includes('.m3u8')) {
            console.log('M3U8 detected via XHR:', url);
            showUI(url, 'network');
        }
        return originalOpen.apply(this, arguments);
    };

    // Scan for video tags periodically
    setInterval(() => {
        const videos = document.getElementsByTagName('video');
        for (let video of videos) {
            if (video.src && video.src.includes('.m3u8')) {
                // Check if we need to add or update
                // We pass to showUI which handles duplicates
                // console.log('M3U8 detected via video tag:', video.src);
                showUI(video.src, 'video');
            }
            // Check sources inside video
            const sources = video.getElementsByTagName('source');
            for (let source of sources) {
                if (source.src && source.src.includes('.m3u8')) {
                    // console.log('M3U8 detected via source tag:', source.src);
                    showUI(source.src, 'video');
                }
            }
        }
    }, 2000);

})();
