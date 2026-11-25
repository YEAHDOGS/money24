import Plays from './Plays.svelte';
// Shared desk styles: fluid type, springs, the live dot, the range thumb and list scrollbars.
import '../feed/feed.css';
import './plays.css';
import { mountProtected as mount } from '../lib/app-lock/mount.js';

export default mount(Plays, { target: document.getElementById('plays') });
