import 'bootstrap/dist/css/bootstrap.css';
import './css/bootstrap-theme.css';
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
         faTasks, faTerminal, faCompress, faExpand, faEyeSlash, faStop, faExclamation,
         faColumns, faDownload, faSync, faCheck, faHourglass, faArchive,
         faSort, faSortUp, faSortDown, faPause, faPlay, faCopy,
         faWindowClose, faPencilAlt, faArrowUp, faArrowDown, faPlus, faSave, faTrash,
         faBinoculars, faUpload, faExternalLinkAlt, faTags, faTimes, faFolder,
         faSignOutAlt, faBroom, faPaperPlane, faEdit, faChevronDown, faFileDownload,
         faEraser, faFileCsv, faFileImport,
       } from '@fortawesome/free-solid-svg-icons';

library.add(faHome, faCrosshairs, faWrench, faEye, faServer, faBook, faLaptop,
            faSearch, faSpinner, faSearchPlus, faTasks, faTerminal,
            faCompress, faExpand, faEyeSlash, faStop, faExclamation,
            faColumns, faDownload, faSync, faCheck, faHourglass, faArchive,
            faSort, faSortUp, faSortDown, faPause, faPlay, faCopy,
            faWindowClose, faPencilAlt, faArrowUp, faArrowDown, faPlus, faSave, faTrash,
            faFolderOpen , faHistory, faBinoculars, faUpload, faExternalLinkAlt,
            faTags, faTimes, faFolder, faSignOutAlt, faBroom, faPaperPlane, faEdit,
            faChevronDown, faFileDownload, faEraser, faFileCsv, faFileImport,
           );

ReactDOM.render(
    <HashRouter>
        <App />
    </HashRouter>,
    document.getElementById('root')
);

// If you want your app to work offline and load faster, you can change
// unregister() to register() below. Note this comes with some pitfalls.
// Learn more about service workers: https://bit.ly/CRA-PWA
serviceWorker.unregister();
