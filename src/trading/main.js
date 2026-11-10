import Trading from './Trading.svelte';
// Shared desk styles: fluid type, springs, the live dot and list scrollbars.
import '../feed/feed.css';
import { mount } from 'svelte';

export default mount(Trading, { target: document.getElementById('trading') });
