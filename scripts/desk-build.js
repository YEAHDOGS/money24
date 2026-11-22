// One build configuration for the PC supervisor and the headless desk.
export const DESK_PATH = '/desk';
export const WEB_ENV = {
  M24_BASE: `${DESK_PATH}/`,
  VITE_TRADING_API: DESK_PATH,
  VITE_NEWS_API: `${DESK_PATH}/news`,
  VITE_SELF_HOSTED: '1',
};
