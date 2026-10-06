/**
 * MoneyPath - High Performance ezBookkeeping Geotag Route Animator
 */

(function() {
  'use strict';

  // --- State Variables ---
  const state = {
    points: [], // Array of MoneyPathPoint
    visiblePoints: [],
    filterCache: new Map(),
    markerSetCacheKey: null,
    lastRenderedPointKey: null,
    allCategories: [],
    allTags: [],
    selectedCategories: [],
    selectedTags: [],
    categoryFilterMode: 'all',
    tagFilterMode: 'all',
    searchText: '',
    savedFilters: [],
    markerVisibility: {
      expense: true,
      income: true,
      transfer: true
    },
    totalPoints: 0,
    totalDistanceKm: 0,
    totalExpense: 0,
    primaryCurrency: '₹',
    
    // Animation Engine State
    isPlaying: false,
    currentIndex: 0,       // integer index of last completed point
    progressFraction: 0,   // continuous 0.0 to (totalPoints - 1)
    animationDurationSec: 30, // Default fixed phase duration
    speedMultiplier: 1.0,
    lastFrameTime: null,
    animationFrameId: null,
    mode: 'fixed',         // 'fixed' or 'realtime'
    followCamera: true,
    useClustering: false,
    useHeatmap: false,
    heatOptions: {
      radius: 25,
      blur: 18,
      intensityScale: 1.0
    },
    heatFilterDays: 0,
    // Marker Sizes (Expense, Income, Transfer)
    markerSizes: {
      expense: 8,
      income: 8,
      transfer: 8
    },
    // Filter & Tile State
    datePreset: 'all',
    startDate: null,
    endDate: null,
    currentTileLayer: null,
    selectedMapStyle: 'osm'
  };

  // load/save marker size options helpers
  function loadMarkerSizeOptions() {
    try {
      const raw = localStorage.getItem('moneypath_markerSizes');
      if (raw) {
        const parsed = JSON.parse(raw);
        if (parsed && typeof parsed === 'object') {
          if (parsed.expense) state.markerSizes.expense = Number(parsed.expense) || 8;
          if (parsed.income) state.markerSizes.income = Number(parsed.income) || 8;
          if (parsed.transfer) state.markerSizes.transfer = Number(parsed.transfer) || 8;
        }
      }
      const rawStyle = localStorage.getItem('moneypath_mapStyle');
      if (rawStyle && (rawStyle === 'osm' || rawStyle === 'satellite' || rawStyle === 'carto')) {
        state.selectedMapStyle = rawStyle;
      } else {
        state.selectedMapStyle = 'osm';
      }
    } catch (e) {}

    updateMarkerSizeControlsUI();
  }

  function saveMarkerSizeOptions() {
    try {
      localStorage.setItem('moneypath_markerSizes', JSON.stringify(state.markerSizes));
    } catch (e) {}
  }

  function updateMarkerSizeControlsUI() {
    if (el.sliderSizeExpense) {
      el.sliderSizeExpense.value = state.markerSizes.expense;
      if (el.valSizeExpense) el.valSizeExpense.textContent = `${state.markerSizes.expense}px`;
    }
    if (el.sliderSizeIncome) {
      el.sliderSizeIncome.value = state.markerSizes.income;
      if (el.valSizeIncome) el.valSizeIncome.textContent = `${state.markerSizes.income}px`;
    }
    if (el.sliderSizeTransfer) {
      el.sliderSizeTransfer.value = state.markerSizes.transfer;
      if (el.valSizeTransfer) el.valSizeTransfer.textContent = `${state.markerSizes.transfer}px`;
    }
    if (el.sliderSizeMaster) {
      const avg = Math.round((state.markerSizes.expense + state.markerSizes.income + state.markerSizes.transfer) / 3);
      el.sliderSizeMaster.value = avg;
      if (el.valSizeMaster) el.valSizeMaster.textContent = `${avg}px`;
    }
  }

  // load/save heat options helpers
  function loadHeatOptions() {
    try {
      const raw = localStorage.getItem('moneypath_heatOptions');
      if (raw) {
        const parsed = JSON.parse(raw);
        if (parsed && typeof parsed === 'object') Object.assign(state.heatOptions, parsed);
      }
      const rawFilter = localStorage.getItem('moneypath_heatFilterDays');
      if (rawFilter) state.heatFilterDays = Number(rawFilter) || 0;
    } catch (e) {}

    if (el.heatRadiusSlider) el.heatRadiusSlider.value = state.heatOptions.radius;
    if (el.heatBlurSlider) el.heatBlurSlider.value = state.heatOptions.blur;
    if (el.heatIntensitySlider) el.heatIntensitySlider.value = state.heatOptions.intensityScale;
    if (el.heatRadiusValue) el.heatRadiusValue.textContent = el.heatRadiusSlider.value;
    if (el.heatBlurValue) el.heatBlurValue.textContent = el.heatBlurSlider.value;
    if (el.heatIntensityValue) el.heatIntensityValue.textContent = Number(el.heatIntensitySlider.value).toFixed(1);
    if (el.heatRangeSelect) el.heatRangeSelect.value = String(state.heatFilterDays || 0);
  }

  function saveHeatOptions() {
    try { localStorage.setItem('moneypath_heatOptions', JSON.stringify(state.heatOptions)); } catch (e) {}
    try { localStorage.setItem('moneypath_heatFilterDays', String(state.heatFilterDays || 0)); } catch (e) {}
  }

  function matchesVisibleType(type) {
    if (type === 2) return !!state.markerVisibility.income;
    if (type === 4) return !!state.markerVisibility.transfer;
    return !!state.markerVisibility.expense;
  }

  function getDisplayPoints() {
    if (Array.isArray(state.visiblePoints) && state.visiblePoints.length >= 0) {
      return state.visiblePoints;
    }
    return state.points || [];
  }

  function getFilterCacheKey() {
    return JSON.stringify({
      minTime: state.minTime ?? null,
      maxTime: state.maxTime ?? null,
      searchText: (state.searchText || '').trim().toLowerCase(),
      markerVisibility: {
        expense: !!state.markerVisibility.expense,
        income: !!state.markerVisibility.income,
        transfer: !!state.markerVisibility.transfer
      },
      selectedCategories: [...(state.selectedCategories || [])].slice().sort(),
      selectedTags: [...(state.selectedTags || [])].slice().sort(),
      categoryFilterMode: state.categoryFilterMode || 'all',
      tagFilterMode: state.tagFilterMode || 'all'
    });
  }

  function applyMapFilters() {
    const cacheKey = getFilterCacheKey();
    if (state.filterCache.has(cacheKey)) {
      state.visiblePoints = state.filterCache.get(cacheKey).slice();
    } else {
      const search = (state.searchText || '').trim().toLowerCase();
      const filtered = (state.points || []).filter((point) => {
        if (!matchesVisibleType(point.type)) return false;

        if (state.categoryFilterMode === 'none') return false;
        if (state.categoryFilterMode === 'custom' && state.selectedCategories.length > 0 && !state.selectedCategories.includes(point.categoryName)) return false;

        if (state.tagFilterMode === 'none') return false;
        if (state.tagFilterMode === 'custom' && state.selectedTags.length > 0) {
          const tags = (point.tags || []).map(tag => String(tag).trim());
          const hasSelectedTag = tags.some(tag => state.selectedTags.includes(tag));
          if (!hasSelectedTag) return false;
        }

        if (search && !(point.comment || '').toLowerCase().includes(search)) return false;
        return true;
      });

      state.filterCache.set(cacheKey, filtered.slice());
      state.visiblePoints = filtered.slice();
    }

    const filtered = state.visiblePoints;
    state.totalPoints = filtered.length;
    if (filtered.length === 0) {
      el.slider.value = 0;
      el.slider.max = 0;
      el.timeCurrentStart.textContent = '--';
      el.timeTotalEnd.textContent = '--';
      el.hudAmount.textContent = formatCurrency(0);
      el.statPointsCount.textContent = '0';
      renderMarkers();
      updateHeatmap();
      return;
    }

    const start = filtered[0];
    const end = filtered[filtered.length - 1];
    el.slider.min = 0;
    el.slider.max = filtered.length - 1;
    el.timeCurrentStart.textContent = start.formattedTime || '--';
    el.timeTotalEnd.textContent = end.formattedTime || '--';
    renderMarkers();
    updateHeatmap();
    renderFrameAtFraction(0);
  }

  function syncFilterModes() {
    const categoryCount = state.allCategories.length;
    const tagCount = state.allTags.length;

    if (categoryCount === 0) {
      state.categoryFilterMode = 'all';
    } else if (state.categoryFilterMode === 'none') {
      state.categoryFilterMode = 'none';
    } else if (state.selectedCategories.length === 0) {
      state.categoryFilterMode = 'none';
    } else if (state.selectedCategories.length === categoryCount) {
      state.categoryFilterMode = 'all';
    } else {
      state.categoryFilterMode = 'custom';
    }

    if (tagCount === 0) {
      state.tagFilterMode = 'all';
    } else if (state.tagFilterMode === 'none') {
      state.tagFilterMode = 'none';
    } else if (state.selectedTags.length === 0) {
      state.tagFilterMode = 'none';
    } else if (state.selectedTags.length === tagCount) {
      state.tagFilterMode = 'all';
    } else {
      state.tagFilterMode = 'custom';
    }
  }

  function renderFilterOptionList(containerId, values, selectedValues, kind) {
    const container = document.getElementById(containerId);
    if (!container) return;
    container.innerHTML = '';

    if (!values || values.length === 0) {
      const empty = document.createElement('span');
      empty.className = 'filter-empty';
      empty.textContent = 'No values available';
      container.appendChild(empty);
      return;
    }

    values.forEach((value) => {
      const label = document.createElement('label');
      label.className = 'filter-pill';

      const checkbox = document.createElement('input');
      checkbox.type = 'checkbox';
      checkbox.checked = selectedValues.includes(value);
      checkbox.value = value;
      checkbox.addEventListener('change', (event) => {
        const checked = event.target.checked;
        if (kind === 'category') {
          if (checked && !state.selectedCategories.includes(value)) state.selectedCategories.push(value);
          if (!checked) state.selectedCategories = state.selectedCategories.filter(item => item !== value);
          state.categoryFilterMode = state.selectedCategories.length === 0 ? 'none' : (state.selectedCategories.length === state.allCategories.length ? 'all' : 'custom');
        } else {
          if (checked && !state.selectedTags.includes(value)) state.selectedTags.push(value);
          if (!checked) state.selectedTags = state.selectedTags.filter(item => item !== value);
          state.tagFilterMode = state.selectedTags.length === 0 ? 'none' : (state.selectedTags.length === state.allTags.length ? 'all' : 'custom');
        }
        applyMapFilters();
      });

      const text = document.createElement('span');
      text.textContent = value;

      label.appendChild(checkbox);
      label.appendChild(text);
      container.appendChild(label);
    });
  }

  function syncCategoryAndTagFilters() {
    const categories = [...new Set((state.points || []).map(point => point.categoryName).filter(Boolean))].sort();
    const tags = [...new Set((state.points || []).flatMap(point => Array.isArray(point.tags) ? point.tags : []).filter(Boolean))].sort();
    state.allCategories = categories;
    state.allTags = tags;

    if (state.categoryFilterMode === 'all' && state.selectedCategories.length === 0) {
      state.selectedCategories = categories.slice();
    } else {
      state.selectedCategories = state.selectedCategories.filter(item => categories.includes(item));
    }

    if (state.tagFilterMode === 'all' && state.selectedTags.length === 0) {
      state.selectedTags = tags.slice();
    } else {
      state.selectedTags = state.selectedTags.filter(item => tags.includes(item));
    }

    syncFilterModes();
    renderFilterOptionList('category-filter-list', categories, state.selectedCategories, 'category');
    renderFilterOptionList('tag-filter-list', tags, state.selectedTags, 'tag');
  }

  function loadSavedFilters() {
    try {
      const raw = localStorage.getItem('moneypath_saved_filters');
      if (!raw) {
        state.savedFilters = [];
        return;
      }
      const parsed = JSON.parse(raw);
      state.savedFilters = Array.isArray(parsed) ? parsed : [];
    } catch (e) {
      state.savedFilters = [];
    }
  }

  function saveSavedFilters() {
    try {
      localStorage.setItem('moneypath_saved_filters', JSON.stringify(state.savedFilters || []));
    } catch (e) {}
  }

  function renderSavedFilters() {
    const container = document.getElementById('saved-filter-list');
    if (!container) return;
    container.innerHTML = '';

    if (!state.savedFilters || state.savedFilters.length === 0) {
      const empty = document.createElement('span');
      empty.className = 'filter-empty';
      empty.textContent = 'No saved filters yet';
      container.appendChild(empty);
      return;
    }

    state.savedFilters.forEach((preset) => {
      const row = document.createElement('div');
      row.className = 'saved-filter-row';

      const applyBtn = document.createElement('button');
      applyBtn.type = 'button';
      applyBtn.className = 'saved-filter-chip';
      applyBtn.textContent = preset.name || 'Untitled Filter';
      applyBtn.title = 'Apply saved filter';
      applyBtn.addEventListener('click', () => {
        state.datePreset = preset.datePreset || 'custom';
        state.minTime = preset.minTime ?? null;
        state.maxTime = preset.maxTime ?? null;
        state.startDate = preset.startDate || null;
        state.endDate = preset.endDate || null;
        state.markerVisibility = { ...state.markerVisibility, ...(preset.markerVisibility || {}) };
        state.selectedCategories = Array.isArray(preset.selectedCategories) ? preset.selectedCategories.slice() : state.allCategories.slice();
        state.selectedTags = Array.isArray(preset.selectedTags) ? preset.selectedTags.slice() : state.allTags.slice();
        state.categoryFilterMode = Array.isArray(preset.selectedCategories) && preset.selectedCategories.length === 0 ? 'none' : (Array.isArray(preset.selectedCategories) && preset.selectedCategories.length === state.allCategories.length ? 'all' : 'custom');
        state.tagFilterMode = Array.isArray(preset.selectedTags) && preset.selectedTags.length === 0 ? 'none' : (Array.isArray(preset.selectedTags) && preset.selectedTags.length === state.allTags.length ? 'all' : 'custom');
        state.searchText = preset.searchText || '';

        document.querySelectorAll('[data-type-filter]').forEach((checkbox) => {
          const key = checkbox.dataset.typeFilter;
          checkbox.checked = !!state.markerVisibility[key];
        });

        if (el.inputSearchDescription) el.inputSearchDescription.value = state.searchText;
        if (el.inputDateStart) el.inputDateStart.value = preset.startDate || '';
        if (el.inputDateEnd) el.inputDateEnd.value = preset.endDate || '';
        if (preset.startDate && preset.endDate) {
          el.labelDateRange.textContent = `${preset.startDate} ~ ${preset.endDate}`;
        } else if (preset.startDate) {
          el.labelDateRange.textContent = `From ${preset.startDate}`;
        } else if (preset.endDate) {
          el.labelDateRange.textContent = `Until ${preset.endDate}`;
        } else {
          el.labelDateRange.textContent = 'All Time';
        }

        syncCategoryAndTagFilters();
        applyMapFilters();
        el.modalDateFilter.classList.remove('active');
      });

      const deleteBtn = document.createElement('button');
      deleteBtn.type = 'button';
      deleteBtn.className = 'saved-filter-delete';
      deleteBtn.textContent = '×';
      deleteBtn.title = 'Delete saved filter';
      deleteBtn.addEventListener('click', (event) => {
        event.stopPropagation();
        state.savedFilters = state.savedFilters.filter(item => item.id !== preset.id);
        saveSavedFilters();
        renderSavedFilters();
      });

      row.appendChild(applyBtn);
      row.appendChild(deleteBtn);
      container.appendChild(row);
    });
  }

  // load heat options from localStorage on startup
  try { loadHeatOptions(); } catch (e) {}
  try { loadSavedFilters(); } catch (e) {}
  try { renderSavedFilters(); } catch (e) {}

  // --- DOM Elements ---
  const el = {
    map: null,
    canvasRenderer: null,
    routePolyline: null,
    activeMarker: null,
    circleMarkersLayer: null,

    // Controls
    btnPlayPause: document.getElementById('btn-play-pause'),
    iconPlayState: document.getElementById('icon-play-state'),
    btnStepPrev: document.getElementById('btn-step-prev'),
    btnStepNext: document.getElementById('btn-step-next'),
    btnReset: document.getElementById('btn-reset'),
    slider: document.getElementById('timeline-slider'),
    selectFixedPhase: document.getElementById('select-fixed-phase'),
    selectSpeed: document.getElementById('select-speed'),
    btnToggleFollow: document.getElementById('btn-toggle-follow'),
    btnFitBounds: document.getElementById('btn-fit-bounds'),
    selectMapStyle: document.getElementById('select-map-style'),
    btnRefresh: document.getElementById('btn-refresh'),
    btnLogout: document.getElementById('btn-logout'),
    btnMarkerSize: document.getElementById('btn-marker-size'),
    panelMarkerSize: document.getElementById('panel-marker-size'),
    btnCloseMarkerPanel: document.getElementById('btn-close-marker-panel'),
    sliderSizeMaster: document.getElementById('slider-size-master'),
    valSizeMaster: document.getElementById('val-size-master'),
    sliderSizeExpense: document.getElementById('slider-size-expense'),
    valSizeExpense: document.getElementById('val-size-expense'),
    sliderSizeIncome: document.getElementById('slider-size-income'),
    valSizeIncome: document.getElementById('val-size-income'),
    sliderSizeTransfer: document.getElementById('slider-size-transfer'),
    valSizeTransfer: document.getElementById('val-size-transfer'),
    btnResetMarkerSizes: document.getElementById('btn-reset-marker-sizes'),
    btnLegendMarkerSize: document.getElementById('btn-legend-marker-size'),
    btnToggleCluster: document.getElementById('btn-toggle-cluster'),
    btnToggleHeat: document.getElementById('btn-toggle-heat'),
    heatControls: document.getElementById('heat-controls'),
    heatRadiusSlider: document.getElementById('heat-radius-slider'),
    heatBlurSlider: document.getElementById('heat-blur-slider'),
    heatIntensitySlider: document.getElementById('heat-intensity-slider'),
    heatRadiusValue: document.getElementById('heat-radius-value'),
    heatBlurValue: document.getElementById('heat-blur-value'),
    heatIntensityValue: document.getElementById('heat-intensity-value'),
    heatRangeSelect: document.getElementById('heat-range-select'),
    // Time Readouts
    timeCurrentStart: document.getElementById('time-current-start'),
    timeTotalEnd: document.getElementById('time-total-end'),

    // HUD
    hudCurrency: document.getElementById('hud-currency'),
    hudAmount: document.getElementById('hud-amount'),
    statPointsCount: document.getElementById('stat-points-count'),
    statDistance: document.getElementById('stat-distance'),

    // Active Card
    activePointCard: document.getElementById('active-point-card'),
    cardCategoryBadge: document.getElementById('card-category-badge'),
    cardCategoryText: document.getElementById('card-category-text'),
    cardTime: document.getElementById('card-time'),
    cardAmount: document.getElementById('card-amount'),
    cardComment: document.getElementById('card-comment'),
    cardAccount: document.getElementById('card-account'),
    cardCoords: document.getElementById('card-coords'),

    // Modals
    btnDateFilter: document.getElementById('btn-date-filter'),
    labelDateRange: document.getElementById('label-date-range'),
    modalDateFilter: document.getElementById('modal-date-filter'),
    btnCloseDateModal: document.getElementById('btn-close-date-modal'),
    btnCancelDate: document.getElementById('btn-cancel-date'),
    btnApplyDate: document.getElementById('btn-apply-date'),
    inputDateStart: document.getElementById('input-date-start'),
    inputDateEnd: document.getElementById('input-date-end'),
    datePresetButtons: document.querySelectorAll('.date-preset-btn'),
    inputSearchDescription: document.getElementById('input-search-description'),
    btnSaveCurrentFilter: document.getElementById('btn-save-current-filter'),
    btnClearSavedFilters: document.getElementById('btn-clear-saved-filters'),
    btnClearFilters: document.getElementById('btn-clear-filters'),
    btnSelectAllCategories: document.getElementById('btn-select-all-categories'),
    btnClearCategories: document.getElementById('btn-clear-categories'),
    btnSelectAllTags: document.getElementById('btn-select-all-tags'),
    btnClearTags: document.getElementById('btn-clear-tags'),

    // Export Modal
    btnExportMenu: document.getElementById('btn-export-menu'),
    modalExport: document.getElementById('modal-export'),
    btnCloseExportModal: document.getElementById('btn-close-export-modal'),
    btnExportStandaloneHtml: document.getElementById('btn-export-standalone-html'),
    btnExportGeojson: document.getElementById('btn-export-geojson'),
    btnExportCsv: document.getElementById('btn-export-csv'),

    toastContainer: document.getElementById('toast-container'),

    // Node Detail Modal Elements
    modalNodeDetail: document.getElementById('modal-node-detail'),
    nodeModalTitle: document.getElementById('node-modal-title'),
    nodeModalAmount: document.getElementById('node-modal-amount'),
    nodeModalDateTime: document.getElementById('node-modal-datetime'),
    nodeModalAddress: document.getElementById('node-modal-address'),
    nodeModalCategory: document.getElementById('node-modal-category'),
    nodeModalComment: document.getElementById('node-modal-comment'),
    nodeModalAccount: document.getElementById('node-modal-account'),
    nodeModalCoords: document.getElementById('node-modal-coords'),
    btnNodeExportCsv: document.getElementById('btn-node-export-csv'),
    btnNodeExportGeojson: document.getElementById('btn-node-export-geojson'),
    // btnNodeEdit removed (not implemented)
    btnNodeClose: document.getElementById('btn-node-close')
  };

  // --- Tile Layer Definitions ---
  const tileProviders = {
    osm: {
      url: 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',
      subdomains: 'abc',
      maxNativeZoom: 19,
      maxZoom: 22,
      className: 'osm-dark-tiles',
      attribution: 'Map data © <a href="https://openstreetmap.org" target="_blank" style="color:inherit; text-decoration:underline;">OpenStreetMap</a> contributors'
    },
    carto: {
      // CARTO raster Voyager basemap (recommended for raster tiles).
      // The server-provided key will be appended at runtime as `key=` and we
      // add a cache-busting `_cb` query param when the layer is applied.
      url: 'https://basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}.png',
      subdomains: 'abcd',
      maxNativeZoom: 19,
      maxZoom: 22,
      className: 'carto-tiles',
      attribution: 'Tiles © Carto'
    },
    satellite: {
      url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
      subdomains: '',
      maxNativeZoom: 19,
      maxZoom: 22,
      className: '',
      attribution: 'Tiles © Esri'
    }
  };

  // Server-side configuration (populated from /config endpoint)
  let serverConfig = {};
  // Fetch server config (contains CARTO API key when set) and update Carto provider URL if needed
  fetch('/config').then(r => r.json()).then(cfg => {
    serverConfig = cfg || {};
    try {
      if (serverConfig.carto_api_key) {
        // Append API key as a query param named `key` to Carto basemaps as required by CARTO
        tileProviders.carto.url = `https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png?key=${serverConfig.carto_api_key}`;
        // If the map is already initialized and Carto is the selected style, refresh the tile layer
        if (state.selectedMapStyle === 'carto' && el.map) {
          try { setTileLayer('carto'); } catch (e) {}
        }
      }
      if (serverConfig.default_animation_duration) {
        state.animationDurationSec = Number(serverConfig.default_animation_duration) || state.animationDurationSec;
      }
    } catch (e) {
      // ignore
    }
  }).catch(() => {});

  // --- Map Initialization ---
  function initMap() {
    // Leaflet Canvas renderer for maximum 60fps rendering performance with 3000+ coordinates
    el.canvasRenderer = L.canvas({ padding: 0.5, tolerance: 5 });

    // Center on Bangalore by default
    el.map = L.map('map', {
      center: [12.9716, 77.5946],
      zoom: 12,
      minZoom: 2,
      maxZoom: 22,
      zoomControl: false,
      renderer: el.canvasRenderer,
      attributionControl: true
    });

    // Custom positioned zoom control
    L.control.zoom({ position: 'bottomright' }).addTo(el.map);

    setTileLayer(state.selectedMapStyle || 'osm');

    // Route polyline with neon glowing cyan style
    el.routePolyline = L.polyline([], {
      color: '#38bdf8',
      weight: 3.5,
      opacity: 0.9,
      lineJoin: 'round',
      lineCap: 'round',
      renderer: el.canvasRenderer
    }).addTo(el.map);

    // Dynamic traveler icon
    const carIcon = L.divIcon({
      className: 'moving-car-marker',
      html: '<div class="moving-car-pulse"></div>',
      iconSize: [24, 24],
      iconAnchor: [12, 12]
    });

    el.activeMarker = L.marker([12.9716, 77.5946], { icon: carIcon, zIndexOffset: 1000 });
    el.circleMarkersLayer = L.layerGroup().addTo(el.map);

    // marker clustering group (if MarkerCluster is available)
    if (window.L && window.L.markerClusterGroup) {
      el.clusterGroup = L.markerClusterGroup({
        chunkedLoading: true,
        maxClusterRadius: 40,
        iconCreateFunction: function(cluster) {
          const markers = cluster.getAllChildMarkers();
          const count = markers.length;
          const sum = markers.reduce((s, m) => s + (m.options && m.options.amount ? Number(m.options.amount) : (m.amount || 0)), 0);
          const currency = state.primaryCurrency || '';
          const color = sum > 0 ? '#34d399' : (sum < 0 ? '#f87171' : '#6b7280');
          const html = `<div class="cluster-icon" style="background:${color}"><div class="cluster-count">${count}</div><div class="cluster-sum">${currency}${formatCurrency(sum)}</div></div>`;
          return L.divIcon({ html: html, className: 'custom-cluster-icon', iconSize: L.point(48, 48) });
        }
      });
      state.useClustering = false;
    } else {
      el.clusterGroup = null;
      state.useClustering = false;
    }

    // Heatmap layer
    el.heatLayer = null;
    state.useHeatmap = false;
  }

  function setTileLayer(key) {
    if (!tileProviders[key]) {
      key = 'osm';
    }
    state.selectedMapStyle = key;
    try {
      localStorage.setItem('moneypath_mapStyle', key);
    } catch (e) {}

    if (el.selectMapStyle) {
      el.selectMapStyle.value = key;
    }

    if (el.currentTileLayer) {
      el.map.removeLayer(el.currentTileLayer);
    }

    const provider = tileProviders[key];
    // Build tile URL and options. For CARTO, append server key and cache-bust param.
    let tileUrl = provider.url;
    if (key === 'carto' && serverConfig && serverConfig.carto_api_key) {
      const sep = tileUrl.includes('?') ? '&' : '?';
      tileUrl = `${tileUrl}${sep}key=${serverConfig.carto_api_key}&_cb=${Date.now()}`;
    }

    const tileOptions = {
      maxZoom: provider.maxZoom || 22,
      maxNativeZoom: provider.maxNativeZoom || 19,
      attribution: provider.attribution || ''
    };
    if (provider.subdomains) {
      tileOptions.subdomains = provider.subdomains;
    }
    if (provider.className) {
      tileOptions.className = provider.className;
    }

    el.currentTileLayer = L.tileLayer(tileUrl, tileOptions).addTo(el.map);

    // Update compact attribution footer if present
    const attrEl = document.getElementById('map-attribution');
    if (attrEl) {
      attrEl.innerHTML = provider.attribution || '';
      attrEl.style.display = provider.attribution ? 'block' : 'none';
    }
  }

  function getMarkerRadius(type) {
    if (type === 2) return state.markerSizes.income || 8;
    if (type === 4) return state.markerSizes.transfer || 8;
    return state.markerSizes.expense || 8;
  }

  function getMarkerSetCacheKey() {
    const points = getDisplayPoints();
    const lastPoint = points.length > 0 ? points[points.length - 1] : null;
    const firstPoint = points.length > 0 ? points[0] : null;
    return JSON.stringify({
      useClustering: !!state.useClustering,
      count: points.length,
      first: firstPoint ? `${firstPoint.id || ''}-${firstPoint.latitude}-${firstPoint.longitude}` : 'none',
      last: lastPoint ? `${lastPoint.id || ''}-${lastPoint.latitude}-${lastPoint.longitude}` : 'none',
      sizes: {
        expense: state.markerSizes.expense,
        income: state.markerSizes.income,
        transfer: state.markerSizes.transfer
      }
    });
  }

  function syncMapOverlayLayers() {
    if (state.useClustering && el.clusterGroup) {
      if (el.map.hasLayer(el.circleMarkersLayer)) el.map.removeLayer(el.circleMarkersLayer);
      if (!el.map.hasLayer(el.clusterGroup)) el.map.addLayer(el.clusterGroup);
    } else {
      if (el.clusterGroup && el.map.hasLayer(el.clusterGroup)) el.map.removeLayer(el.clusterGroup);
      if (!el.map.hasLayer(el.circleMarkersLayer)) el.map.addLayer(el.circleMarkersLayer);
    }

    if (state.useHeatmap && el.heatLayer) {
      if (!el.map.hasLayer(el.heatLayer)) el.heatLayer.addTo(el.map);
    } else if (el.heatLayer && el.map.hasLayer(el.heatLayer)) {
      el.map.removeLayer(el.heatLayer);
    }
  }

  function renderMarkers() {
    const points = getDisplayPoints();
    if (!points || points.length === 0) return;

    const markerKey = getMarkerSetCacheKey();
    if (state.markerSetCacheKey === markerKey) {
      syncMapOverlayLayers();
      return;
    }
    state.markerSetCacheKey = markerKey;

    if (el.clusterGroup) el.clusterGroup.clearLayers();
    if (el.circleMarkersLayer) el.circleMarkersLayer.clearLayers();

    points.forEach((p) => {
      let color = '#f87171'; // expense (3)
      if (p.type === 2) color = '#34d399'; // income
      if (p.type === 4) color = '#fbbf24'; // transfer

      const radius = getMarkerRadius(p.type);
      const tooltipText = `${p.formattedTime || ''} — ${p.categoryName || ''} — ${getCurrencySymbol(p.currency||state.primaryCurrency)}${formatCurrency(p.amount)}`;

      if (state.useClustering && el.clusterGroup) {
        const dotSize = Math.max(10, radius * 2);
        const html = `<span style="display:inline-block;width:${dotSize}px;height:${dotSize}px;border-radius:50%;background:${color};border:2px solid rgba(255,255,255,0.85);box-shadow:0 0 4px rgba(0,0,0,0.5);"></span>`;
        const m = L.marker([p.latitude, p.longitude], {
          icon: L.divIcon({
            className: 'cluster-dot',
            html: html,
            iconSize: [dotSize + 6, dotSize + 6],
            iconAnchor: [(dotSize + 6) / 2, (dotSize + 6) / 2]
          })
        });
        m.on('click', () => openNodeModal(p));
        try { m.options.amount = Number(p.amount) || 0; } catch (e) { m.options.amount = 0; }
        m.options.txType = p.type;
        m.bindTooltip(tooltipText, { direction: 'top', offset: [0, -8], opacity: 0.95 });
        el.clusterGroup.addLayer(m);
      } else {
        const cm = L.circleMarker([p.latitude, p.longitude], {
          radius: radius,
          fillColor: color,
          color: '#ffffff',
          weight: 1.5,
          opacity: 0.9,
          fillOpacity: 0.95,
          renderer: el.canvasRenderer
        });
        cm.pointData = p;
        cm.bindTooltip(tooltipText, { direction: 'top', offset: [0, -8], opacity: 0.95 });
        cm.on('click', () => openNodeModal(p));
        cm.addTo(el.circleMarkersLayer);
      }
    });

    syncMapOverlayLayers();
  }

  function updateCircleMarkerSizes() {
    if (state.useClustering) {
      renderMarkers();
    } else if (el.circleMarkersLayer) {
      el.circleMarkersLayer.eachLayer(layer => {
        if (layer.pointData) {
          layer.setRadius(getMarkerRadius(layer.pointData.type));
        }
      });
    }
  }

  // --- Data Loading & Processing ---
  async function loadTransactions(options = {}) {
    showToast('Fetching transactions...', 'info');
    let url = '/api/transactions';
    const params = new URLSearchParams();

    if (options.useMock) {
      url = '/api/mock';
      params.set('count', options.mockCount || 3000);
    } else {
      if (state.minTime) {
        params.set('min_time', state.minTime);
      }
      if (state.maxTime) {
        params.set('max_time', state.maxTime);
      }
    }

    const fullUrl = `${url}?${params.toString()}`;

    try {
      const response = await fetch(fullUrl);
      if (!response.ok) {
        throw new Error(`HTTP error ${response.status}`);
      }
      const data = await response.json();

      if (!data.success) {
        throw new Error(data.errorMessage || 'Failed to load data from ezBookkeeping');
      }

      setDataset(data);
      showToast(`Loaded ${data.totalWithGeo.toLocaleString()} geotagged points!`, 'success');
    } catch (err) {
      console.warn('Live API request failed:', err);
      showToast(`Could not connect to ezBookkeeping: ${err.message}`, 'warning');
    }
  }

  function setDataset(data) {
    pauseAnimation();

    state.filterCache.clear();
    state.points = data.points || [];
    state.visiblePoints = state.points.slice();
    state.totalPoints = state.points.length;
    state.totalDistanceKm = data.totalDistanceKm || 0;
    state.totalExpense = data.totalExpense || 0;
    
    // Currency mapping with support for Indian Rupee (INR)
    state.primaryCurrency = getCurrencySymbol(data.primaryCurrency);

    const categories = [...new Set(state.points.map(point => point.categoryName).filter(Boolean))].sort();
    const tags = [...new Set(state.points.flatMap(point => Array.isArray(point.tags) ? point.tags : []).filter(Boolean))].sort();
    state.allCategories = categories;
    state.allTags = tags;
    state.selectedCategories = state.selectedCategories.length ? state.selectedCategories.filter(item => categories.includes(item)) : categories.slice();
    state.selectedTags = state.selectedTags.length ? state.selectedTags.filter(item => tags.includes(item)) : tags.slice();
    syncCategoryAndTagFilters();
    if (el.inputSearchDescription) {
      el.inputSearchDescription.value = state.searchText || '';
    }
    applyMapFilters();

    el.hudCurrency.textContent = state.primaryCurrency;
    el.statPointsCount.textContent = state.totalPoints.toLocaleString();
    el.statDistance.textContent = `${state.totalDistanceKm.toFixed(1)} km`;

    if (state.totalPoints === 0) {
      resetPlayback();
      showToast('No geotagged transactions found for selected criteria.', 'info');
      return;
    }

    // Update Slider Bounds
    el.slider.min = 0;
    el.slider.max = state.totalPoints - 1;
    el.slider.value = 0;

    // Update Time range HUD
    el.timeCurrentStart.textContent = state.points[0].formattedTime || '--';
    el.timeTotalEnd.textContent = state.points[state.totalPoints - 1].formattedTime || '--';

    // Render markers with current size and clustering settings
    renderMarkers();

    // After markers created, update heatmap if needed
    updateHeatmap();

    // Fit Map Bounds
    const latLngs = state.points.map(p => [p.latitude, p.longitude]);
    const bounds = L.latLngBounds(latLngs);
    if (bounds.isValid()) {
      el.map.fitBounds(bounds, { padding: [50, 50], maxZoom: 18 });
    }

    resetPlayback();
    renderFrameAtFraction(0);
  }

  // --- Core Animation Engine ---
  function startAnimation() {
    if (state.totalPoints < 2) return;
    if (state.progressFraction >= state.totalPoints - 1) {
      // Reached the end, loop back to start
      state.progressFraction = 0;
    }
    state.isPlaying = true;
    state.lastFrameTime = performance.now();
    updatePlayPauseUI();
    state.animationFrameId = requestAnimationFrame(animationLoop);
  }

  function pauseAnimation() {
    state.isPlaying = false;
    if (state.animationFrameId) {
      cancelAnimationFrame(state.animationFrameId);
      state.animationFrameId = null;
    }
    updatePlayPauseUI();
  }

  function togglePlayPause() {
    if (state.isPlaying) {
      pauseAnimation();
    } else {
      startAnimation();
    }
  }

  function resetPlayback() {
    pauseAnimation();
    state.progressFraction = 0;
    state.currentIndex = 0;
    el.slider.value = 0;
    renderFrameAtFraction(0);
  }

  function animationLoop(timestamp) {
    if (!state.isPlaying) return;

    const deltaMs = timestamp - (state.lastFrameTime || timestamp);
    state.lastFrameTime = timestamp;

    let pointsAdvanced = 0;
    if (state.mode === 'fixed') {
      // Fixed total duration: points per second = totalPoints / duration
      const pointsPerSec = (state.totalPoints / state.animationDurationSec) * state.speedMultiplier;
      pointsAdvanced = (pointsPerSec * (deltaMs / 1000));
    } else {
      // Real-time mode: map real-world time elapsed
      const totalSpanSec = (state.points[state.totalPoints - 1].timestamp - state.points[0].timestamp) || 1;
      const speed = 100 * state.speedMultiplier; // 100x baseline
      const realSecAdvanced = (deltaMs / 1000) * speed;
      pointsAdvanced = (realSecAdvanced / totalSpanSec) * state.totalPoints;
    }

    state.progressFraction += pointsAdvanced;

    if (state.progressFraction >= state.totalPoints - 1) {
      state.progressFraction = state.totalPoints - 1;
      renderFrameAtFraction(state.progressFraction);
      pauseAnimation();
      showToast('Trajectory animation completed!', 'info');
      return;
    }

    renderFrameAtFraction(state.progressFraction);
    state.animationFrameId = requestAnimationFrame(animationLoop);
  }

  // Linear interpolation between two coordinates
  function interpolateCoordinate(p1, p2, ratio) {
    return [
      p1.latitude + (p2.latitude - p1.latitude) * ratio,
      p1.longitude + (p2.longitude - p1.longitude) * ratio
    ];
  }

  // Render a specific frame at progress fraction (e.g. 142.6)
  function renderFrameAtFraction(fraction) {
    const points = getDisplayPoints();
    if (!points || points.length === 0) return;

    const total = points.length;
    const clampedFraction = Math.max(0, Math.min(fraction, total - 1));
    const floorIndex = Math.floor(clampedFraction);
    const subProgress = clampedFraction - floorIndex;

    const currentPoint = points[floorIndex];
    let activePos = [currentPoint.latitude, currentPoint.longitude];

    if (floorIndex < total - 1 && subProgress > 0) {
      const nextPoint = points[floorIndex + 1];
      activePos = interpolateCoordinate(currentPoint, nextPoint, subProgress);
    }

    const drawnLatLngs = [];
    for (let i = 0; i <= floorIndex; i++) {
      drawnLatLngs.push([points[i].latitude, points[i].longitude]);
    }
    if (subProgress > 0 && floorIndex < total - 1) {
      drawnLatLngs.push(activePos);
    }

    const pointKey = currentPoint.id || `${currentPoint.timestamp || floorIndex}-${currentPoint.latitude}-${currentPoint.longitude}`;
    if (state.lastRenderedPointKey !== pointKey) {
      state.lastRenderedPointKey = pointKey;
      updateActiveCard(currentPoint);
    }

    el.routePolyline.setLatLngs(drawnLatLngs);

    if (!el.map.hasLayer(el.activeMarker)) {
      el.activeMarker.addTo(el.map);
    }
    el.activeMarker.setLatLng(activePos);

    if (state.followCamera) {
      el.map.panTo(activePos, { animate: false });
    }

    el.slider.value = floorIndex;
    el.timeCurrentStart.textContent = currentPoint.formattedTime || '--';

    let runningSpend = 0;
    for (let i = 0; i <= floorIndex; i++) {
      const p = points[i];
      if (!p) continue;
      if (p.type === 3) {
        const amt = Number(p.amount || 0);
        runningSpend += Math.abs(amt);
      }
    }
    el.hudAmount.textContent = formatCurrency(runningSpend);
  }

  function getCurrencySymbol(curr) {
    if (!curr || curr === 'INR') return '₹';
    if (curr === 'USD') return '$';
    if (curr === 'EUR') return '€';
    if (curr === 'GBP') return '£';
    if (curr === 'JPY') return '¥';
    return curr + ' ';
  }

  function formatCurrency(val) {
    return (val || 0).toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }

  function updateActiveCard(point) {
    if (!point) return;
    el.activePointCard.classList.add('visible');
    el.cardCategoryText.textContent = point.categoryName || 'General';
    el.cardCategoryBadge.style.backgroundColor = point.categoryColor || '#38bdf8';
    el.cardTime.textContent = point.formattedTime || '';
    const symbol = getCurrencySymbol(point.currency || state.primaryCurrency);
    el.cardAmount.textContent = `${symbol}${formatCurrency(point.amount)}`;
    el.cardAmount.style.color = point.type === 3 ? '#f87171' : (point.type === 2 ? '#34d399' : '#38bdf8');
    el.cardComment.textContent = point.comment || 'No comment provided';
    // If both source and destination accounts are present, show From → To. Otherwise show single account.
    try {
      const from = point.sourceAccountName || '';
      const to = point.destinationAccountName || '';
      if (from && to && from !== to) {
        el.cardAccount.textContent = `From: ${from} → To: ${to} (${point.typeName})`;
      } else if (from) {
        el.cardAccount.textContent = `${from} (${point.typeName})`;
      } else if (to) {
        el.cardAccount.textContent = `${to} (${point.typeName})`;
      } else {
        el.cardAccount.textContent = `${point.accountName} (${point.typeName})`;
      }
    } catch (e) {
      el.cardAccount.textContent = `${point.accountName} (${point.typeName})`;
    }
    el.cardCoords.textContent = `Lat: ${point.latitude.toFixed(5)}, Lon: ${point.longitude.toFixed(5)}`;
  }

  function updateHeatmap() {
    if (!window.L || !window.L.heatLayer) return;
    const points = getDisplayPoints();
    if (!points || points.length === 0) {
      if (el.heatLayer) {
        try { el.map.removeLayer(el.heatLayer); } catch (e) {}
        el.heatLayer = null;
      }
      if (el.heatControls) el.heatControls.style.display = 'none';
      return;
    }

    if (!state.useHeatmap) {
      if (el.heatLayer) {
        try { el.map.removeLayer(el.heatLayer); } catch (e) {}
        el.heatLayer = null;
      }
      if (el.heatControls) el.heatControls.style.display = 'none';
      return;
    }

    const nowSec = Math.floor(Date.now() / 1000);
    const days = Number(state.heatFilterDays || 0);
    let expensePoints = points.filter(p => p.type === 3 && p.latitude && p.longitude && (p.amount !== undefined));
    if (days > 0) {
      const cutoff = nowSec - (days * 24 * 60 * 60);
      expensePoints = expensePoints.filter(p => (p.timestamp || p.Time || 0) >= cutoff);
    }
    if (expensePoints.length === 0) {
      if (el.heatLayer) {
        try { el.map.removeLayer(el.heatLayer); } catch (e) {}
        el.heatLayer = null;
      }
      if (el.heatControls) el.heatControls.style.display = 'block';
      return;
    }

    const maxAmt = Math.max(...expensePoints.map(p => Math.abs(p.amount) || 1));
    const scale = state.heatOptions.intensityScale || 1.0;
    const heatData = expensePoints.map(p => [p.latitude, p.longitude, Math.min(1, (Math.abs(p.amount) / maxAmt) * scale)]);

    if (el.heatLayer) {
      try { el.map.removeLayer(el.heatLayer); } catch (e) {}
    }

    el.heatLayer = L.heatLayer(heatData, { radius: state.heatOptions.radius || 25, blur: state.heatOptions.blur || 18, maxZoom: 13, gradient: {0.2:'#34d399',0.5:'#fbbf24',1.0:'#f87171'} });
    syncMapOverlayLayers();
    if (el.heatControls) el.heatControls.style.display = 'block';
  }

  // --- Node Detail Modal ---
  function openNodeModal(p) {
    if (!p) return;
    state.modalActivePoint = p;
    el.nodeModalTitle.textContent = p.categoryName || 'Transaction';
    el.nodeModalAmount.textContent = `${getCurrencySymbol(p.currency||state.primaryCurrency)}${formatCurrency(p.amount)}`;
    el.nodeModalCategory.textContent = p.typeName || '';
    el.nodeModalComment.textContent = p.comment || 'No comment';
    // Show both source and destination accounts when available
    try {
      const from = p.sourceAccountName || '';
      const to = p.destinationAccountName || '';
      if (from && to && from !== to) {
        el.nodeModalAccount.textContent = `From: ${from} → To: ${to} (${p.typeName || ''})`;
      } else if (from) {
        el.nodeModalAccount.textContent = `${from} (${p.typeName || ''})`;
      } else if (to) {
        el.nodeModalAccount.textContent = `${to} (${p.typeName || ''})`;
      } else {
        el.nodeModalAccount.textContent = `${p.accountName || ''} (${p.typeName || ''})`;
      }
    } catch (e) {
      el.nodeModalAccount.textContent = `${p.accountName || ''} (${p.typeName || ''})`;
    }
    el.nodeModalCoords.textContent = `Lat: ${p.latitude.toFixed(5)}, Lon: ${p.longitude.toFixed(5)}`;
    // Date/time display (best-effort from timestamp fields)
    const ts = p.timestamp || p.Time || p.time || p.created_at || p.createdAt || 0;
    if (el.nodeModalDateTime) {
      if (ts) {
        try {
          const d = new Date(Number(ts) * 1000);
          el.nodeModalDateTime.textContent = d.toLocaleString();
        } catch (e) {
          el.nodeModalDateTime.textContent = String(ts);
        }
      } else {
        el.nodeModalDateTime.textContent = 'Date: --';
      }
    }

    // Reverse geocode address (cache on point as _address)
    if (el.nodeModalAddress) {
      if (p._address) {
        el.nodeModalAddress.textContent = p._address;
      } else {
        el.nodeModalAddress.textContent = 'Looking up address...';
        const lat = encodeURIComponent(p.latitude);
        const lon = encodeURIComponent(p.longitude);
        const url = `https://nominatim.openstreetmap.org/reverse?format=jsonv2&lat=${lat}&lon=${lon}`;
        fetch(url, { headers: { 'Accept': 'application/json' } })
          .then(r => r.json())
          .then(j => {
            const display = j && (j.display_name || j.error) ? (j.display_name || j.error) : 'Address not found';
            p._address = display;
            if (el.nodeModalAddress) el.nodeModalAddress.textContent = display;
          })
          .catch(() => {
            if (el.nodeModalAddress) el.nodeModalAddress.textContent = 'Address lookup failed';
          });
      }
    }
    if (el.modalNodeDetail) el.modalNodeDetail.classList.add('active');
  }

  function closeNodeModal() {
    state.modalActivePoint = null;
    if (el.modalNodeDetail) el.modalNodeDetail.classList.remove('active');
  }

  function updatePlayPauseUI() {
    // Use small inline SVGs for reliable swapping regardless of lucide state
    const iconEl = document.getElementById('icon-play-state');
    if (!iconEl) return;

    const playSvg = '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M5 3v18l15-9z"></path></svg>';
    const pauseSvg = '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"></path></svg>';

    // Replace the icon element with a simple span that contains the inline SVG
    const wrapper = document.createElement('span');
    wrapper.id = 'icon-play-state';
    wrapper.className = 'icon-inline';
    wrapper.innerHTML = state.isPlaying ? pauseSvg : playSvg;

    iconEl.replaceWith(wrapper);
  }

  // --- Step & Scrubber Controls ---
  function stepTo(newIndex) {
    pauseAnimation();
    const target = Math.max(0, Math.min(newIndex, state.totalPoints - 1));
    state.progressFraction = target;
    renderFrameAtFraction(target);
  }

  // --- Export Utilities ---
  function exportStandaloneHTML() {
    const rawDataJson = JSON.stringify({
      points: state.points,
      totalDistanceKm: state.totalDistanceKm,
      totalExpense: state.totalExpense,
      primaryCurrency: state.primaryCurrency
    });

    // Use CARTO raster Voyager export URL and include key when available.
    const exportTileUrl = (state.selectedMapStyle === 'carto' && serverConfig && serverConfig.carto_api_key)
      ? `https://basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}.png?key=${serverConfig.carto_api_key}`
      : 'https://basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}.png';

    const htmlContent = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>MoneyPath Exported Route (${state.points.length} Coordinates)</title>
  <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"/>
  <style>
    body, html, #map { margin:0; padding:0; height:100%; width:100%; background:#0f172a; font-family:sans-serif; }
    .hud { position:absolute; top:20px; left:20px; z-index:1000; background:rgba(15,23,42,0.85); backdrop-filter:blur(8px); padding:16px; border-radius:12px; color:#fff; border:1px solid rgba(255,255,255,0.1); }
    .dock { position:absolute; bottom:20px; left:50%; transform:translateX(-50%); z-index:1000; background:rgba(15,23,42,0.85); backdrop-filter:blur(8px); padding:12px 24px; border-radius:12px; display:flex; gap:12px; align-items:center; color:#fff; }
    button { background:#0284c7; color:#fff; border:none; padding:8px 16px; border-radius:6px; font-weight:bold; cursor:pointer; }
    .pulse { width:20px; height:20px; background:#38bdf8; border:2px solid #fff; border-radius:50%; box-shadow:0 0 12px #38bdf8; }
  </style>
</head>
<body>
  <div id="map"></div>
  <div class="hud">
    <h3 style="margin:0 0 4px 0;">MoneyPath Financial Route</h3>
    <div>Total Points: <b>${state.points.length}</b></div>
    <div>Total Distance: <b>${state.totalDistanceKm.toFixed(1)} km</b></div>
  </div>
  <div class="dock">
    <button id="btnPlay">Play / Pause</button>
    <input type="range" id="slider" min="0" max="${state.points.length - 1}" value="0" style="width:300px;">
  </div>
  <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
  <script>
    const data = ${rawDataJson};
    const pts = data.points;
    const map = L.map('map', { renderer: L.canvas() }).setView([pts[0].latitude, pts[0].longitude], 13);
    L.tileLayer('${exportTileUrl}').addTo(map);
    const poly = L.polyline([], { color: '#38bdf8', weight: 4 }).addTo(map);
    const marker = L.marker([pts[0].latitude, pts[0].longitude], {
      icon: L.divIcon({ html: '<div class="pulse"></div>', iconSize:[20,20], iconAnchor:[10,10] })
    }).addTo(map);
    map.fitBounds(L.latLngBounds(pts.map(p => [p.latitude, p.longitude])));

    let idx = 0, playing = false, reqId = null;
    function render(i) {
      poly.setLatLngs(pts.slice(0, i+1).map(p => [p.latitude, p.longitude]));
      marker.setLatLng([pts[i].latitude, pts[i].longitude]);
      document.getElementById('slider').value = i;
    }
    function loop() {
      if (!playing) return;
      idx = (idx + 1) % pts.length;
      render(idx);
      reqId = setTimeout(() => requestAnimationFrame(loop), 20);
    }
    document.getElementById('btnPlay').onclick = () => {
      playing = !playing;
      if (playing) loop(); else clearTimeout(reqId);
    };
    document.getElementById('slider').oninput = (e) => {
      playing = false;
      idx = parseInt(e.target.value);
      render(idx);
    };
    render(0);
  </script>
</body>
</html>`;

    downloadFile(htmlContent, 'moneypath_route_visualization.html', 'text/html');
    showToast('Exported Standalone HTML map!', 'success');
  }

  function exportGeoJSON() {
    const featureCollection = {
      type: 'FeatureCollection',
      features: [
        {
          type: 'Feature',
          properties: {
            name: 'MoneyPath Sequential Trajectory',
            totalPoints: state.points.length,
            totalDistanceKm: state.totalDistanceKm
          },
          geometry: {
            type: 'LineString',
            coordinates: state.points.map(p => [p.longitude, p.latitude])
          }
        },
        ...state.points.map(p => ({
          type: 'Feature',
          properties: {
            id: p.id,
            timestamp: p.timestamp,
            formattedTime: p.formattedTime,
            category: p.categoryName,
            account: p.accountName,
            amount: p.amount,
            comment: p.comment
          },
          geometry: {
            type: 'Point',
            coordinates: [p.longitude, p.latitude]
          }
        }))
      ]
    };

    downloadFile(JSON.stringify(featureCollection, null, 2), 'moneypath_trail.geojson', 'application/geo+json');
    showToast('Exported GeoJSON file!', 'success');
  }

  function exportCSV() {
    const headers = ['ID', 'Timestamp', 'FormattedTime', 'Latitude', 'Longitude', 'Category', 'Account', 'Amount', 'Currency', 'Comment'];
    const rows = state.points.map(p => [
      p.id,
      p.timestamp,
      `"${p.formattedTime}"`,
      p.latitude,
      p.longitude,
      `"${p.categoryName}"`,
      `"${p.accountName}"`,
      p.amount,
      p.currency,
      `"${(p.comment || '').replace(/"/g, '""')}"`
    ]);

    const csvContent = [headers.join(','), ...rows.map(r => r.join(','))].join('\n');
    downloadFile(csvContent, 'moneypath_transactions.csv', 'text/csv');
    showToast('Exported CSV file!', 'success');
  }

  function downloadFile(content, fileName, mimeType) {
    const blob = new Blob([content], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = fileName;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  // --- UI Toast Helpers ---
  function showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.textContent = message;
    el.toastContainer.appendChild(toast);
    setTimeout(() => {
      toast.style.opacity = '0';
      setTimeout(() => toast.remove(), 300);
    }, 3500);
  }

  // --- Event Listeners Setup ---
  function setupEventListeners() {
    // Playback
    el.btnPlayPause.addEventListener('click', togglePlayPause);
    el.btnReset.addEventListener('click', resetPlayback);
    el.btnStepPrev.addEventListener('click', () => stepTo(Math.floor(state.progressFraction) - 1));
    el.btnStepNext.addEventListener('click', () => stepTo(Math.floor(state.progressFraction) + 1));

    // Scrubber
    el.slider.addEventListener('input', (e) => {
      pauseAnimation();
      const val = parseInt(e.target.value, 10);
      state.progressFraction = val;
      renderFrameAtFraction(val);
    });

    // Fixed Phase Duration selector
    el.selectFixedPhase.addEventListener('change', (e) => {
      const val = e.target.value;
      if (val === 'realtime') {
        state.mode = 'realtime';
      } else {
        state.mode = 'fixed';
        state.animationDurationSec = parseFloat(val);
      }
    });

    // Speed multiplier
    el.selectSpeed.addEventListener('change', (e) => {
      state.speedMultiplier = parseFloat(e.target.value);
    });

    // Camera Follow Toggle
    el.btnToggleFollow.addEventListener('click', () => {
      state.followCamera = !state.followCamera;
      el.btnToggleFollow.classList.toggle('btn-active', state.followCamera);
      showToast(state.followCamera ? 'Camera follow enabled' : 'Camera follow disabled', 'info');
    });

    // Cluster Toggle
    if (el.btnToggleCluster) {
      el.btnToggleCluster.addEventListener('click', () => {
        if (!el.clusterGroup) {
          showToast('Marker clustering is unavailable in this browser/session.', 'warning');
          return;
        }
        state.useClustering = !state.useClustering;
        el.btnToggleCluster.classList.toggle('btn-active', state.useClustering);
        state.markerSetCacheKey = null;
        renderMarkers();
        syncMapOverlayLayers();
      });
    }

    // Heatmap Toggle
    if (el.btnToggleHeat) {
      el.btnToggleHeat.addEventListener('click', () => {
        state.useHeatmap = !state.useHeatmap;
        el.btnToggleHeat.classList.toggle('btn-active', state.useHeatmap);
        updateHeatmap();
        syncMapOverlayLayers();
      });
    }

    // Heat sliders
    if (el.heatRadiusSlider) {
      el.heatRadiusSlider.addEventListener('input', (ev) => {
        const v = Number(ev.target.value);
        state.heatOptions.radius = v;
        if (el.heatRadiusValue) el.heatRadiusValue.textContent = v;
        saveHeatOptions();
        if (state.useHeatmap) updateHeatmap();
      });
    }
    if (el.heatBlurSlider) {
      el.heatBlurSlider.addEventListener('input', (ev) => {
        const v = Number(ev.target.value);
        state.heatOptions.blur = v;
        if (el.heatBlurValue) el.heatBlurValue.textContent = v;
        saveHeatOptions();
        if (state.useHeatmap) updateHeatmap();
      });
    }
    if (el.heatIntensitySlider) {
      el.heatIntensitySlider.addEventListener('input', (ev) => {
        const v = Number(ev.target.value);
        state.heatOptions.intensityScale = v;
        if (el.heatIntensityValue) el.heatIntensityValue.textContent = Number(v).toFixed(1);
        saveHeatOptions();
        if (state.useHeatmap) updateHeatmap();
      });
    }

    // Heat range select
    if (el.heatRangeSelect) {
      el.heatRangeSelect.addEventListener('change', (ev) => {
        const v = Number(ev.target.value || 0);
        state.heatFilterDays = v;
        saveHeatOptions();
        if (state.useHeatmap) updateHeatmap();
      });
    }

    // Marker Size Panel Toggle & Sliders
    if (el.btnMarkerSize) {
      el.btnMarkerSize.addEventListener('click', (e) => {
        e.stopPropagation();
        const isOpen = el.panelMarkerSize && el.panelMarkerSize.style.display !== 'none';
        if (el.panelMarkerSize) el.panelMarkerSize.style.display = isOpen ? 'none' : 'flex';
        el.btnMarkerSize.classList.toggle('btn-active', !isOpen);
      });
    }

    if (el.btnLegendMarkerSize) {
      el.btnLegendMarkerSize.addEventListener('click', (e) => {
        e.stopPropagation();
        if (el.panelMarkerSize) el.panelMarkerSize.style.display = 'flex';
        if (el.btnMarkerSize) el.btnMarkerSize.classList.add('btn-active');
      });
    }

    if (el.btnCloseMarkerPanel) {
      el.btnCloseMarkerPanel.addEventListener('click', () => {
        if (el.panelMarkerSize) el.panelMarkerSize.style.display = 'none';
        if (el.btnMarkerSize) el.btnMarkerSize.classList.remove('btn-active');
      });
    }

    // Close marker size panel when clicking outside
    document.addEventListener('click', (e) => {
      if (el.panelMarkerSize && el.panelMarkerSize.style.display !== 'none') {
        const inPanel = el.panelMarkerSize.contains(e.target);
        const inBtn = el.btnMarkerSize && el.btnMarkerSize.contains(e.target);
        const inLegendBtn = el.btnLegendMarkerSize && el.btnLegendMarkerSize.contains(e.target);
        if (!inPanel && !inBtn && !inLegendBtn) {
          el.panelMarkerSize.style.display = 'none';
          if (el.btnMarkerSize) el.btnMarkerSize.classList.remove('btn-active');
        }
      }
    });

    // Master Size Slider
    if (el.sliderSizeMaster) {
      el.sliderSizeMaster.addEventListener('input', (e) => {
        const val = parseInt(e.target.value, 10);
        state.markerSizes.expense = val;
        state.markerSizes.income = val;
        state.markerSizes.transfer = val;
        updateMarkerSizeControlsUI();
        saveMarkerSizeOptions();
        updateCircleMarkerSizes();
      });
    }

    // Expense Size Slider
    if (el.sliderSizeExpense) {
      el.sliderSizeExpense.addEventListener('input', (e) => {
        const val = parseInt(e.target.value, 10);
        state.markerSizes.expense = val;
        updateMarkerSizeControlsUI();
        saveMarkerSizeOptions();
        updateCircleMarkerSizes();
      });
    }

    // Income Size Slider
    if (el.sliderSizeIncome) {
      el.sliderSizeIncome.addEventListener('input', (e) => {
        const val = parseInt(e.target.value, 10);
        state.markerSizes.income = val;
        updateMarkerSizeControlsUI();
        saveMarkerSizeOptions();
        updateCircleMarkerSizes();
      });
    }

    // Transfer Size Slider
    if (el.sliderSizeTransfer) {
      el.sliderSizeTransfer.addEventListener('input', (e) => {
        const val = parseInt(e.target.value, 10);
        state.markerSizes.transfer = val;
        updateMarkerSizeControlsUI();
        saveMarkerSizeOptions();
        updateCircleMarkerSizes();
      });
    }

    // Reset Marker Sizes
    if (el.btnResetMarkerSizes) {
      el.btnResetMarkerSizes.addEventListener('click', () => {
        state.markerSizes.expense = 8;
        state.markerSizes.income = 8;
        state.markerSizes.transfer = 8;
        updateMarkerSizeControlsUI();
        saveMarkerSizeOptions();
        updateCircleMarkerSizes();
        showToast('Marker sizes reset to default (8px)', 'info');
      });
    }

    // Fit Bounds
    el.btnFitBounds.addEventListener('click', () => {
      if (state.points.length > 0) {
        const bounds = L.latLngBounds(state.points.map(p => [p.latitude, p.longitude]));
        el.map.fitBounds(bounds, { padding: [50, 50], maxZoom: 18 });
      }
    });

    // Map Style Switcher
    el.selectMapStyle.addEventListener('change', (e) => {
      setTileLayer(e.target.value);
    });

    // Refresh
    el.btnRefresh.addEventListener('click', () => loadTransactions());

    // Logout
    if (el.btnLogout) {
      el.btnLogout.addEventListener('click', () => {
        window.location.href = '/logout';
      });
    }

    // Date Filter Modal
    el.btnDateFilter.addEventListener('click', () => el.modalDateFilter.classList.add('active'));
    el.btnCloseDateModal.addEventListener('click', () => el.modalDateFilter.classList.remove('active'));
    el.btnCancelDate.addEventListener('click', () => el.modalDateFilter.classList.remove('active'));

    el.datePresetButtons.forEach(btn => {
      btn.addEventListener('click', () => {
        const preset = btn.dataset.preset;
        state.datePreset = preset;
        const now = new Date();

        if (preset === 'all') {
          state.minTime = null;
          state.maxTime = null;
          state.startDate = null;
          state.endDate = null;
          el.inputDateStart.value = '';
          el.inputDateEnd.value = '';
          el.labelDateRange.textContent = 'All Time';
          el.modalDateFilter.classList.remove('active');
          loadTransactions();
        } else if (preset === '7d') {
          const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 7, 0, 0, 0);
          const end = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59);
          state.minTime = Math.floor(start.getTime() / 1000);
          state.maxTime = Math.floor(end.getTime() / 1000);
          el.labelDateRange.textContent = 'Last 7 Days';
          el.modalDateFilter.classList.remove('active');
          loadTransactions();
        } else if (preset === '30d') {
          const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 30, 0, 0, 0);
          const end = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59);
          state.minTime = Math.floor(start.getTime() / 1000);
          state.maxTime = Math.floor(end.getTime() / 1000);
          el.labelDateRange.textContent = 'Last 30 Days';
          el.modalDateFilter.classList.remove('active');
          loadTransactions();
        } else if (preset === 'month') {
          const start = new Date(now.getFullYear(), now.getMonth(), 1, 0, 0, 0);
          const end = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59);
          state.minTime = Math.floor(start.getTime() / 1000);
          state.maxTime = Math.floor(end.getTime() / 1000);
          el.labelDateRange.textContent = 'This Month';
          el.modalDateFilter.classList.remove('active');
          loadTransactions();
        } else if (preset === 'year') {
          const start = new Date(now.getFullYear(), 0, 1, 0, 0, 0);
          const end = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59);
          state.minTime = Math.floor(start.getTime() / 1000);
          state.maxTime = Math.floor(end.getTime() / 1000);
          el.labelDateRange.textContent = 'This Year';
          el.modalDateFilter.classList.remove('active');
          loadTransactions();
        } else if (preset === 'custom') {
          el.inputDateStart.focus();
        }
      });
    });

    el.btnApplyDate.addEventListener('click', () => {
      const startStr = el.inputDateStart.value;
      const endStr = el.inputDateEnd.value;

      if (startStr) {
        const parts = startStr.split('-').map(Number);
        state.minTime = Math.floor(new Date(parts[0], parts[1] - 1, parts[2], 0, 0, 0).getTime() / 1000);
      } else {
        state.minTime = null;
      }

      if (endStr) {
        const parts = endStr.split('-').map(Number);
        state.maxTime = Math.floor(new Date(parts[0], parts[1] - 1, parts[2], 23, 59, 59).getTime() / 1000);
      } else {
        state.maxTime = null;
      }

      if (startStr && endStr) {
        el.labelDateRange.textContent = `${startStr} ~ ${endStr}`;
      } else if (startStr) {
        el.labelDateRange.textContent = `From ${startStr}`;
      } else if (endStr) {
        el.labelDateRange.textContent = `Until ${endStr}`;
      } else {
        el.labelDateRange.textContent = 'All Time';
      }

      el.modalDateFilter.classList.remove('active');
      loadTransactions();
    });

    document.querySelectorAll('[data-type-filter]').forEach((checkbox) => {
      checkbox.addEventListener('change', (event) => {
        const val = event.target.dataset.typeFilter;
        if (val === 'expense') state.markerVisibility.expense = event.target.checked;
        if (val === 'income') state.markerVisibility.income = event.target.checked;
        if (val === 'transfer') state.markerVisibility.transfer = event.target.checked;
        applyMapFilters();
      });
    });

    if (el.inputSearchDescription) {
      el.inputSearchDescription.addEventListener('input', (event) => {
        state.searchText = (event.target.value || '').trim();
        applyMapFilters();
      });
    }

    if (el.btnSaveCurrentFilter) {
      el.btnSaveCurrentFilter.addEventListener('click', () => {
        const name = window.prompt('Name this saved filter:', `Filter ${state.savedFilters.length + 1}`);
        const trimmed = (name || '').trim();
        const preset = {
          id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
          name: trimmed || `Filter ${state.savedFilters.length + 1}`,
          datePreset: state.datePreset || 'custom',
          startDate: el.inputDateStart && el.inputDateStart.value ? el.inputDateStart.value : '',
          endDate: el.inputDateEnd && el.inputDateEnd.value ? el.inputDateEnd.value : '',
          minTime: state.minTime ?? null,
          maxTime: state.maxTime ?? null,
          markerVisibility: {
            expense: !!state.markerVisibility.expense,
            income: !!state.markerVisibility.income,
            transfer: !!state.markerVisibility.transfer
          },
          selectedCategories: state.selectedCategories.slice(),
          selectedTags: state.selectedTags.slice(),
          searchText: state.searchText || ''
        };

        state.savedFilters = [...state.savedFilters, preset];
        saveSavedFilters();
        renderSavedFilters();
      });
    }

    if (el.btnClearSavedFilters) {
      el.btnClearSavedFilters.addEventListener('click', () => {
        state.savedFilters = [];
        saveSavedFilters();
        renderSavedFilters();
      });
    }

    if (el.btnClearFilters) {
      el.btnClearFilters.addEventListener('click', () => {
        state.markerVisibility.expense = true;
        state.markerVisibility.income = true;
        state.markerVisibility.transfer = true;
        state.selectedCategories = state.allCategories.slice();
        state.selectedTags = state.allTags.slice();
        state.categoryFilterMode = 'all';
        state.tagFilterMode = 'all';
        state.searchText = '';
        state.minTime = null;
        state.maxTime = null;
        state.startDate = null;
        state.endDate = null;
        state.datePreset = 'all';
        if (el.inputDateStart) el.inputDateStart.value = '';
        if (el.inputDateEnd) el.inputDateEnd.value = '';
        el.labelDateRange.textContent = 'All Time';
        document.querySelectorAll('[data-type-filter]').forEach((checkbox) => {
          checkbox.checked = true;
        });
        if (el.inputSearchDescription) el.inputSearchDescription.value = '';
        syncCategoryAndTagFilters();
        applyMapFilters();
      });
    }

    if (el.btnSelectAllCategories) {
      el.btnSelectAllCategories.addEventListener('click', () => {
        state.selectedCategories = state.allCategories.slice();
        state.categoryFilterMode = 'all';
        syncCategoryAndTagFilters();
        applyMapFilters();
      });
    }
    if (el.btnClearCategories) {
      el.btnClearCategories.addEventListener('click', () => {
        state.selectedCategories = [];
        state.categoryFilterMode = 'none';
        syncCategoryAndTagFilters();
        applyMapFilters();
      });
    }
    if (el.btnSelectAllTags) {
      el.btnSelectAllTags.addEventListener('click', () => {
        state.selectedTags = state.allTags.slice();
        state.tagFilterMode = 'all';
        syncCategoryAndTagFilters();
        applyMapFilters();
      });
    }
    if (el.btnClearTags) {
      el.btnClearTags.addEventListener('click', () => {
        state.selectedTags = [];
        state.tagFilterMode = 'none';
        syncCategoryAndTagFilters();
        applyMapFilters();
      });
    }

    // Export Modal
    el.btnExportMenu.addEventListener('click', () => el.modalExport.classList.add('active'));
    el.btnCloseExportModal.addEventListener('click', () => el.modalExport.classList.remove('active'));
    el.btnExportStandaloneHtml.addEventListener('click', exportStandaloneHTML);
    el.btnExportGeojson.addEventListener('click', exportGeoJSON);
    el.btnExportCsv.addEventListener('click', exportCSV);

    // Node detail modal actions
    if (el.btnNodeClose) el.btnNodeClose.addEventListener('click', closeNodeModal);
    if (el.btnNodeExportCsv) el.btnNodeExportCsv.addEventListener('click', () => {
      const p = state.modalActivePoint;
      if (!p) return;
      const url = new URL('/export/transaction/csv', window.location.origin);
      url.searchParams.set('id', p.id);
      if (state.minTime) url.searchParams.set('min_time', state.minTime);
      if (state.maxTime) url.searchParams.set('max_time', state.maxTime);
      // Navigate to the download endpoint
      window.location.href = url.toString();
    });
    if (el.btnNodeExportGeojson) el.btnNodeExportGeojson.addEventListener('click', () => {
      const p = state.modalActivePoint;
      if (!p) return;
      const url = new URL('/export/transaction/geojson', window.location.origin);
      url.searchParams.set('id', p.id);
      if (state.minTime) url.searchParams.set('min_time', state.minTime);
      if (state.maxTime) url.searchParams.set('max_time', state.maxTime);
      window.location.href = url.toString();
    });
    // Edit button removed; no-op

    // Keyboard Shortcuts (Space for Play/Pause, Left/Right for Step)
    window.addEventListener('keydown', (e) => {
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
      if (e.code === 'Space') {
        e.preventDefault();
        togglePlayPause();
      } else if (e.code === 'ArrowLeft') {
        stepTo(Math.floor(state.progressFraction) - 1);
      } else if (e.code === 'ArrowRight') {
        stepTo(Math.floor(state.progressFraction) + 1);
      }
    });
  }

  // --- Application Bootstrap ---
  function init() {
    loadMarkerSizeOptions();
    loadSavedFilters();
    renderSavedFilters();
    initMap();
    setupEventListeners();
    if (window.lucide) {
      window.lucide.createIcons();
    }
    // Default to Last 30 Days unless user explicitly has another preset saved
    try {
      const now = new Date();
      const end = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59);
      const start = new Date(end.getTime() - (30 * 24 * 60 * 60 * 1000));
      state.datePreset = '30d';
      state.minTime = Math.floor(start.getTime() / 1000);
      state.maxTime = Math.floor(end.getTime() / 1000);
      if (el.labelDateRange) el.labelDateRange.textContent = 'Last 30 Days';
      if (el.inputDateStart) el.inputDateStart.value = start.toISOString().slice(0,10);
      if (el.inputDateEnd) el.inputDateEnd.value = end.toISOString().slice(0,10);
    } catch (e) {}

    // Automatically load transactions
    loadTransactions();
  }

  window.addEventListener('DOMContentLoaded', init);
})();
