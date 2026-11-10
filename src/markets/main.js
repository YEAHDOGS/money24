import Markets from './Markets.svelte';
// Shared desk styles: fluid type, springs, the live dot and list scrollbars.
import '../feed/feed.css';
import { mount } from 'svelte';

export default mount(Markets, { target: document.getElementById('markets') });
