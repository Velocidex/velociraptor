import './css/_variables.css';
import 'bootstrap/dist/css/bootstrap.css';

import React from 'react';
import ReactDOM from 'react-dom';
import './css/index.css';
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
         faEraser, faFileCsv, faFileImport, faMinus, faForward, faCalendarAlt,
         faCompressAlt, faBackward, faMedkit, faVirusSlash, faBookmark, faHeart,
         faFileCode, faFlag, faTrashAlt, faClock, faLock, faLockOpen, faCloud,
         faCloudDownloadAlt, faUserEdit, faFilter, faSortAlphaUp, faSortAlphaDown,
         faInfo, faBug, faUser, faList, faIndent, faTextHeight, faBars,
         faUserLargeSlash, faTriangleExclamation, faCircle, faAnglesLeft, faMaximize,
         faMinimize, faNoteSticky, faArrowsUpDown, faBan, faFileExport,
         faCircleExclamation, faTable, faHouse, faRotateLeft, faRotateRight,
         faChevronRight, faEllipsis, faLayerGroup, faBullseye, faPersonRunning,
         faQuestion, faCalendarPlus, faForwardFast, faBackwardFast, faSliders,
         faRepeat, faBorderAll, faBell, faCircleQuestion, faLightbulb, faBomb,
       } from '@fortawesome/free-solid-svg-icons';

import { faSquare, faSquareCheck, faSquareMinus,
       } from '@fortawesome/free-regular-svg-icons';

library.add(faHome, faCrosshairs, faWrench, faEye, faServer, faBook, faLaptop,
            faSearch, faSpinner, faSearchPlus, faTasks, faTerminal,
            faCompress, faExpand, faEyeSlash, faStop, faExclamation,
            faColumns, faDownload, faSync, faCheck, faHourglass, faArchive,
            faSort, faSortUp, faSortDown, faPause, faPlay, faCopy,
            faWindowClose, faPencilAlt, faArrowUp, faArrowDown, faPlus, faSave, faTrash,
            faFolderOpen , faHistory, faBinoculars, faUpload, faExternalLinkAlt,
            faTags, faTimes, faFolder, faSignOutAlt, faBroom, faPaperPlane, faEdit,
            faChevronDown, faFileDownload, faEraser, faFileCsv, faFileImport, faMinus,
            faForward, faCalendarAlt, faCompressAlt, faBackward, faMedkit, faVirusSlash,
            faBookmark, faHeart, faFileCode, faFlag, faTrashAlt, faClock, faLock, faLockOpen,
            faCloud, faCloudDownloadAlt, faUserEdit, faFilter, faBug,
            faSortAlphaUp, faSortAlphaDown, faInfo, faUser, faList, faIndent,
            faTextHeight, faBars, faUserLargeSlash, faTriangleExclamation,
            faCircle, faAnglesLeft, faMaximize, faMinimize, faNoteSticky,
            faArrowsUpDown, faBan, faFileExport, faCircleExclamation,
            faTable, faHouse, faRotateLeft, faRotateRight, faChevronRight,
            faEllipsis, faLayerGroup, faBullseye, faPersonRunning, faQuestion,
            faCalendarPlus, faForwardFast, faBackwardFast, faSquareCheck,
            faSquare, faSquareMinus, faSliders, faRepeat, faBorderAll, faBell,
            faCircleQuestion, faLightbulb, faBomb,
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
