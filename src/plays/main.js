import Plays from './Plays.svelte';
// Shared desk styles: fluid type, springs, the live dot, the range thumb and list scrollbars.
import '../feed/feed.css';
import './plays.css';
import { mount } from 'svelte';

export default mount(Plays, { target: document.getElementById('plays') });
