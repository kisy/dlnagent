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

    let detectedUrl = '';
    const containerId = 'm3u8-caster-container';
    const CAST_API_URL = 'https://172.16.1.5/api/cast';

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
    `);

    // Create UI
    function createUI() {
        if (document.getElementById(containerId)) return;

        const container = document.createElement('div');
        container.id = containerId;
        container.innerHTML = `
            <div class="close">&times;</div>
            <div class="url"></div>
            <button id="copy-btn" class="copy-btn">Copy</button>
            <button id="cast-btn">Cast to DLNA</button>
        `;
        document.body.appendChild(container);

        container.querySelector('.close').onclick = () => {
            container.style.display = 'none';
        };

        container.querySelector('#copy-btn').onclick = copyLink;
        container.querySelector('#cast-btn').onclick = castVideo;
    }

    // Copy function
    function copyLink() {
        if (!detectedUrl) return;
        navigator.clipboard.writeText(detectedUrl).then(() => {
            alert('Link copied to clipboard!');
        }).catch(err => {
            console.error('Failed to copy: ', err);
            // Fallback for some environments if navigator.clipboard fails
            const textArea = document.createElement("textarea");
            textArea.value = detectedUrl;
            document.body.appendChild(textArea);
            textArea.select();
            try {
                document.execCommand('copy');
                alert('Link copied to clipboard!');
            } catch (err) {
                console.error('Fallback copy failed', err);
                alert('Failed to copy link');
            }
            document.body.removeChild(textArea);
        });
    }

    // Show UI with URL
    function showUI(url) {
        if (!url) return;
        detectedUrl = url;
        createUI();
        const container = document.getElementById(containerId);
        const displayUrl = url.split('?')[0];
        container.querySelector('.url').textContent = 'Detected: ' + displayUrl;
        container.style.display = 'block';
    }

    // Cast function
    function castVideo() {
        if (!detectedUrl) return;

        GM_xmlhttpRequest({
            method: "POST",
            url: CAST_API_URL,
            headers: {
                "Content-Type": "application/json"
            },
            data: JSON.stringify({
                url: detectedUrl,
                title: document.title || "Unknown Title"
            }),
            onload: function(response) {
                console.log("Cast response:", response.responseText);
                if (response.status === 200) {
                    alert('Casting started!');
                } else {
                    alert('Failed to cast: ' + response.responseText);
                }
            },
            onerror: function(err) {
                console.error("Cast error:", err);
                alert('Error connecting to DLNA service');
            }
        });
    }

    // Intercept Fetch
    const originalFetch = window.fetch;
    window.fetch = async function(...args) {
        const [resource, config] = args;
        if (typeof resource === 'string' && resource.includes('.m3u8')) {
            console.log('M3U8 detected via fetch:', resource);
            showUI(resource);
        }
        return originalFetch.apply(this, args);
    };

    // Intercept XHR
    const originalOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(method, url) {
        if (typeof url === 'string' && url.includes('.m3u8')) {
            console.log('M3U8 detected via XHR:', url);
            showUI(url);
        }
        return originalOpen.apply(this, arguments);
    };

    // Scan for video tags periodically
    setInterval(() => {
        const videos = document.getElementsByTagName('video');
        for (let video of videos) {
            if (video.src && video.src.includes('.m3u8')) {
                if (video.src !== detectedUrl) {
                    console.log('M3U8 detected via video tag:', video.src);
                    showUI(video.src);
                }
            }
            // Check sources inside video
            const sources = video.getElementsByTagName('source');
            for (let source of sources) {
                if (source.src && source.src.includes('.m3u8')) {
                    if (source.src !== detectedUrl) {
                        console.log('M3U8 detected via source tag:', source.src);
                        showUI(source.src);
                    }
                }
            }
        }
    }, 2000);

})();
