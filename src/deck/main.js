import { mount } from 'svelte';
import '../feed/feed.css';
import Deck from './Deck.svelte';

const ROOT_ID = 'deck';

export default mount(Deck, { target: document.getElementById(ROOT_ID) });
