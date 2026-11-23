import Pocket from './Pocket.svelte';
// Shared desk styles: fluid type, springs, the live dot, list scrollbars and the rows' rise.
import '../feed/feed.css';
import './pocket.css';
// Light, dark or auto (sunset): applies the saved choice
import '../lib/theme.js';
import { mount } from 'svelte';

export default mount(Pocket, { target: document.getElementById('pocket') });
