import Feed from './Feed.svelte';
import './feed.css';
import { mountProtected as mount } from '../lib/app-lock/mount.js';

export default mount(Feed, { target: document.getElementById('feed') });
