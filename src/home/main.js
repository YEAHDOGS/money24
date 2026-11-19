import Home from './Home.svelte';
// Shared desk styles: fluid type, springs, the live dot and list scrollbars.
import '../feed/feed.css';
import './home.css';
import { mount } from 'svelte';

export default mount(Home, { target: document.getElementById('home') });
