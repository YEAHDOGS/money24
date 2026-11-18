// Whether the work-in-progress notice is open right now. The notice (NoticeBar.svelte) opens and
// closes it; the tour (tour/Tour.svelte) reads it and waits its turn, so a first visit is never two
// things at once. It starts open on a device that has not acknowledged this version of the notice.
import { stored } from './demo.js';

export const STORE_ACK = 'm24.notice.ack';
export const NOTICE_VERSION = '1'; // raise it when the notice changes, so everyone reads it again

export const notice = $state({ open: stored(STORE_ACK, '') !== NOTICE_VERSION });
