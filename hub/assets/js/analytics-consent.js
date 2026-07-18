(function () {
  'use strict';

  var root = document.querySelector('[data-analytics-consent]');
  if (!root) {
    return;
  }

  var acceptButton = root.querySelector('[data-analytics-accept]');
  var rejectButton = root.querySelector('[data-analytics-reject]');
  var statusRegion = root.querySelector('[data-analytics-status]');
  var openSettingsControls = document.querySelectorAll('[data-analytics-open-settings]');

  var storageKey = root.getAttribute('data-storage-key') || 'herbhub365.analytics-consent.v1';
  var measurementId = root.getAttribute('data-ga-id') || '';
  var analyticsEnabled = root.getAttribute('data-enabled') === 'true';
  var allowedHosts = (root.getAttribute('data-allowed-hosts') || '')
    .split(',')
    .map(function (host) { return host.trim().toLowerCase(); })
    .filter(Boolean);
  var hostAllowed = allowedHosts.indexOf(window.location.hostname.toLowerCase()) !== -1;
  var choiceGranted = 'granted';
  var choiceDenied = 'denied';

  var gaDisableKey = measurementId ? 'ga-disable-' + measurementId : '';
  var gaScriptId = 'herbhub365-gtag-script';
  var gaLoaded = false;
  var hasStoredChoice = false;

  function setStatus(message) {
    if (statusRegion) {
      statusRegion.textContent = message || '';
    }
  }

  function readChoice() {
    try {
      var stored = window.localStorage.getItem(storageKey);
      if (stored === choiceGranted || stored === choiceDenied) {
        return stored;
      }
      return null;
    } catch (error) {
      return null;
    }
  }

  function persistChoice(value) {
    try {
      window.localStorage.setItem(storageKey, value);
      return true;
    } catch (error) {
      return false;
    }
  }

  function removeCookie(name, domain) {
    var expires = 'Thu, 01 Jan 1970 00:00:00 GMT';
    var secure = window.location.protocol === 'https:' ? '; Secure' : '';
    var domainPart = domain ? '; domain=' + domain : '';
    document.cookie = name + '=; expires=' + expires + '; path=/' + domainPart + '; SameSite=Lax' + secure;
  }

  function clearGoogleAnalyticsCookies() {
    var cookies = document.cookie ? document.cookie.split(';') : [];
    var cookieNames = [];

    for (var i = 0; i < cookies.length; i += 1) {
      var segment = cookies[i].trim();
      if (!segment) {
        continue;
      }
      var equalsIndex = segment.indexOf('=');
      var cookieName = equalsIndex === -1 ? segment : segment.slice(0, equalsIndex);
      if (cookieName === '_ga' || cookieName.indexOf('_ga_') === 0) {
        cookieNames.push(cookieName);
      }
    }

    if (cookieNames.indexOf('_ga') === -1) {
      cookieNames.push('_ga');
    }
    if (measurementId) {
      var dynamicName = '_ga_' + measurementId.replace(/[^A-Za-z0-9]/g, '_');
      if (cookieNames.indexOf(dynamicName) === -1) {
        cookieNames.push(dynamicName);
      }
    }

    var host = window.location.hostname;
    var domains = [null, host, '.herbhub365.com', 'herbhub365.com'];

    for (var j = 0; j < cookieNames.length; j += 1) {
      for (var k = 0; k < domains.length; k += 1) {
        removeCookie(cookieNames[j], domains[k]);
      }
    }
  }

  function callConsentDeniedIfAvailable() {
    if (typeof window.gtag === 'function') {
      window.gtag('consent', 'update', {
        analytics_storage: 'denied'
      });
    }
  }

  function withdrawAnalytics() {
    if (gaDisableKey) {
      window[gaDisableKey] = true;
    }
    callConsentDeniedIfAvailable();
    clearGoogleAnalyticsCookies();
  }

  function ensureDataLayerAndGtag() {
    if (!Array.isArray(window.dataLayer)) {
      window.dataLayer = [];
    }

    if (typeof window.gtag !== 'function') {
      window.gtag = function () {
        window.dataLayer.push(arguments);
      };
    }
  }

  function loadAnalyticsIfAllowed() {
    if (!analyticsEnabled || !hostAllowed || !measurementId || gaLoaded) {
      return;
    }

    if (gaDisableKey) {
      window[gaDisableKey] = false;
    }

    ensureDataLayerAndGtag();
    window.gtag('js', new Date());
    window.gtag('config', measurementId);

    var existing = document.getElementById(gaScriptId);
    if (existing) {
      gaLoaded = true;
      return;
    }

    var script = document.createElement('script');
    script.async = true;
    script.src = 'https://www.googletagmanager.com/gtag/js?id=' + encodeURIComponent(measurementId);
    script.id = gaScriptId;
    script.onload = function () {
      gaLoaded = true;
    };
    document.head.appendChild(script);
  }

  function openPanel() {
    root.hidden = false;
    if (hasStoredChoice) {
      setStatus('Current setting saved. Choose accept or reject to update.');
    }
    window.requestAnimationFrame(function () {
      root.focus();
    });
  }

  function closePanel() {
    root.hidden = true;
  }

  function applyChoice(value, options) {
    var config = options || {};
    var shouldClose = config.closePanel !== false;
    var saveWorked = persistChoice(value);

    hasStoredChoice = true;

    if (value === choiceGranted) {
      if (gaDisableKey) {
        window[gaDisableKey] = false;
      }
      loadAnalyticsIfAllowed();
      setStatus(saveWorked ? 'Analytics accepted. Your preference is saved.' : 'Analytics accepted for this session. Preference could not be saved.');
    }

    if (value === choiceDenied) {
      withdrawAnalytics();
      setStatus(saveWorked ? 'Analytics rejected. No analytics requests will be made.' : 'Analytics rejected for this session. Preference could not be saved.');
    }

    if (shouldClose) {
      closePanel();
    }
  }

  function wireEvents() {
    if (acceptButton) {
      acceptButton.addEventListener('click', function () {
        applyChoice(choiceGranted);
      });
    }

    if (rejectButton) {
      rejectButton.addEventListener('click', function () {
        applyChoice(choiceDenied);
      });
    }

    for (var i = 0; i < openSettingsControls.length; i += 1) {
      openSettingsControls[i].addEventListener('click', function () {
        openPanel();
      });
    }

    root.addEventListener('keydown', function (event) {
      if (event.key === 'Escape' && hasStoredChoice) {
        closePanel();
      }
    });
  }

  function initialize() {
    if (gaDisableKey) {
      window[gaDisableKey] = true;
    }

    wireEvents();

    var existingChoice = readChoice();
    if (existingChoice === choiceGranted) {
      hasStoredChoice = true;
      closePanel();
      loadAnalyticsIfAllowed();
      return;
    }

    if (existingChoice === choiceDenied) {
      hasStoredChoice = true;
      withdrawAnalytics();
      closePanel();
      return;
    }

    hasStoredChoice = false;
    setStatus('Choose whether analytics is enabled for this site.');
    openPanel();
  }

  initialize();
})();
