import '@testing-library/jest-dom';

// jsdom video elements report readyState 0 / videoWidth 0, which CvEngine's readiness
// guard treats as "no frames yet" (it skips detection). Default every test video to a
// ready, sized state so the suite exercises the detection path; tests that specifically
// cover the not-ready guard override these on the element instance (instance properties
// shadow the prototype getters).
Object.defineProperty(HTMLVideoElement.prototype, 'readyState', {
  configurable: true,
  get: () => 2, // HAVE_CURRENT_DATA
});
Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', {
  configurable: true,
  get: () => 640,
});
