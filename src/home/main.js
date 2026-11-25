import Home from './Home.svelte';
// Shared desk styles: fluid type, springs, the live dot and list scrollbars.
import '../feed/feed.css';
import './home.css';
import { mountProtected as mount } from '../lib/app-lock/mount.js';

export default mount(Home, { target: document.getElementById('home') });
