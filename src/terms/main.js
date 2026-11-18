// The Terms of Use page's only script: it swaps the plain no-JavaScript header for the app header,
// so the terms wear exactly what every app page wears. The text itself never needs it.
import AppHeader from '../lib/AppHeader.svelte';
import { mount } from 'svelte';

const SLOT_ID = 'header-slot';
const slot = document.getElementById(SLOT_ID);

slot.replaceChildren();
export default mount(AppHeader, { target: slot, props: { page: 'terms' } });
