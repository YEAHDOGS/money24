import Pocket from './Pocket.svelte';
// Shared desk styles: fluid type, springs, the live dot, list scrollbars and the rows' rise.
import '../feed/feed.css';
import './pocket.css';
import { mount } from 'svelte';

export default mount(Pocket, { target: document.getElementById('pocket') });
