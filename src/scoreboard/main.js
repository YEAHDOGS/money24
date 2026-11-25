import { mountProtected as mount } from '../lib/app-lock/mount.js';
import '../feed/feed.css';
import Scoreboard from './Scoreboard.svelte';

const ROOT_ID = 'scoreboard';

export default mount(Scoreboard, { target: document.getElementById(ROOT_ID) });
