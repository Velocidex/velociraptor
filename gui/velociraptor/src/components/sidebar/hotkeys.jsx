import React from 'react';
import PropTypes from 'prop-types';

import { GlobalHotKeys } from "react-hotkeys";

import { withRouter }  from "react-router-dom";

class SidebarKeyNavigator extends React.Component {
    static propTypes = {
        client: PropTypes.object,

        // React router props.
        history: PropTypes.object,
    }

    render() {
        return (
            <GlobalHotKeys keyMap={SidebarKeyMap}
                     handlers={{
                         GOTO_DASHBOARD: (e)=> {
                             this.props.history.push("/dashboard");
                             e.preventDefault();
                         },
                         GOTO_ARTIFACTS: (e)=> {
                             this.props.history.push("/artifacts");
                             e.preventDefault();
                         },
                         GOTO_NOTEBOOKS: (e)=> {
                             this.props.history.push("/notebooks");
                             e.preventDefault();
                         },
                         GOTO_COLLECTIONS: (e)=> {
                             let client_id = this.props.client && this.props.client.client_id;
                             if(client_id){
                                 this.props.history.push("/collected/" + client_id);
                             };
                             e.preventDefault();
                         },
                         GOTO_SEARCH: e=>{
                             const el = document.getElementById('client-search-bar');
                             if (el) {
                                 el.focus();
                             }
                         },
                     }}/>
        );
    }
};

export default withRouter(SidebarKeyNavigator);

export const SidebarKeyMap = {
    GOTO_DASHBOARD: {
        name: "Display server dashboard",
        sequence: "alt+d",
    },
    GOTO_ARTIFACTS: {
        name: "Goto artifact view",
        sequence: "alt+a",
    },
    GOTO_NOTEBOOKS: {
        name: "Goto notebooks",
        sequence: "alt+n",
    },
    GOTO_COLLECTIONS: {
        name: "Goto collected artifact",
        sequence: "alt+c",
    },
    GOTO_SEARCH: "ctrl+shift+/",
};
