import 'bootstrap/dist/css/bootstrap.css';
import './css/bootstrap-theme.css';
import 'react-bootstrap-typeahead/css/Typeahead.css';

import './_variables.css';

import React from 'react';
import ReactDOM from 'react-dom';
import './index.css';
import App from './App';
import * as serviceWorker from './serviceWorker';

import {HashRouter} from "react-router-dom";

import { library } from '@fortawesome/fontawesome-svg-core';
import { faHome, faCrosshairs, faWrench, faEye, faServer, faBook, faLaptop,
         faSearch, faSpinner,  faSearchPlus, faFolderOpen, faHistory,
         faTasks, faTerminal,
         faBinoculars } from '@fortawesome/free-solid-svg-icons';

library.add(faHome, faCrosshairs, faWrench, faEye, faServer, faBook, faLaptop,
            faSearch, faSpinner, faSearchPlus, faTasks, faTerminal,
            faFolderOpen , faHistory, faBinoculars);

ReactDOM.render(
    <HashRouter>
        <React.StrictMode>
        <App />
        </React.StrictMode>
    </HashRouter>,
    document.getElementById('root')
);

// If you want your app to work offline and load faster, you can change
// unregister() to register() below. Note this comes with some pitfalls.
// Learn more about service workers: https://bit.ly/CRA-PWA
serviceWorker.unregister();
