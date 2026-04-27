import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import FlowsList from './flows-list.jsx';
import FlowInspector from "./flows-inspector.jsx";
import { withRouter }  from "react-router-dom";
import { HotKeys, ObserveKeys } from "react-hotkeys";
import {getItem, setItem, schema} from '../core/storage.jsx';

import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';

const POLL_TIME = 5000;

class ClientFlowsView extends React.Component {
    static propTypes = {
        client: PropTypes.object,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        currentFlow: {},
    }

    componentDidMount = () => {
        this.source = CancelToken.source();

        let flow_id = this.props.match && this.props.match.params &&
            this.props.match.params.flow_id;

        let client_id = this.props.match && this.props.match.params &&
            this.props.match.params.client_id;
        if(!client_id) {
            client_id = this.props.client.client_id;
        }

        if(client_id && flow_id && flow_id !== "new") {
            this.setState({currentFlow: {
                client_id: client_id,
                session_id: flow_id}});

            this.fetchDetailedFlow(client_id, flow_id);
        }

        // Update the flow object periodically
        this.interval = setInterval(this.fetchDetailedFlow, POLL_TIME);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    fetchDetailedFlow = (client_id, flow_id) => {
        if(!client_id) {
            client_id = this.state.client_id;
        }
        if(!flow_id) {
            flow_id = this.state.flow_id;
        }

        if(!flow_id || !client_id || flow_id === "new") {
            return;
        }

        // Cancel any in flight requests.
        this.source.cancel();
        this.source = CancelToken.source();
        this.setState({loading: true});

        api.get("v1/GetFlowDetails", {
            flow_id: flow_id,
            client_id: client_id,
        }, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            };

            let flow = response.data.context;
            flow.available_downloads = response.data.available_downloads;
            setItem(schema.ClientsCurrentFlowKey, flow_id);
            this.setState({currentFlow: flow,
                           loading: false});
        });
    }

    setSelectedFlow = (flow) => {
        let flow_id = flow.session_id;
        if(!flow_id) {
            return;
        }

        let client_id = this.props.client.client_id;
        if(!client_id) return;

        let tab = this.props.match &&
            this.props.match.params && this.props.match.params.tab;
        // Update the route.
        if (tab) {
            this.props.history.push(
                "/collected/" + client_id + "/" + flow_id + "/" + tab);
        } else {
            this.props.history.push(
                "/collected/" + client_id + "/" + flow_id);
        }

        this.fetchDetailedFlow(client_id, flow_id);
    }

    gotoTab = (tab) => {
        let client_id = this.props.client.client_id;
        if(!client_id) return;

        let selected_flow = this.state.currentFlow &&
            this.state.currentFlow.session_id;
        this.props.history.push(
            "/collected/" + client_id + "/" + selected_flow + "/" + tab);
    }

    collapse = level=> {
        setItem(schema.ClientsSplitKey, level);
        this.setState({topPaneSize: level});
    }

    render() {
        let KeyMap = {
            GOTO_RESULTS: {
                name: "Display server dashboard",
                sequence: "alt+r",
            },
            GOTO_LOGS: "alt+l",
            GOTO_OVERVIEW: "alt+o",
            GOTO_UPLOADS: "alt+u",
            COLLECT: "alt+c",
            NOTEBOOK: "alt+b",
        };

        let keyHandlers={
            GOTO_RESULTS: (e)=>this.gotoTab("results"),
            GOTO_LOGS: (e)=>this.gotoTab("logs"),
            GOTO_UPLOADS: (e)=>this.gotoTab("uploads"),
            GOTO_OVERVIEW: (e)=>this.gotoTab("overview"),
            NOTEBOOK: (e)=>this.gotoTab("notebook"),
            COLLECT: ()=>this.setState({showWizard: true}),
        };

        let selected_flow = this.state.currentFlow;

        return (
            <>
              <HotKeys keyMap={KeyMap} handlers={keyHandlers}>
                <ObserveKeys>
                  <SplitPane split="horizontal"
                             onChange={size=>{
                                 setItem(schema.ClientsSplitKey, size + "px");
                             }}
                             size={getItem(schema.ClientsSplitKey) || "30%"}
                             onResizerDoubleClick={x=>this.collapse("50%")}
                             defaultSize="80%">
                    <FlowsList
                      selected_flow={selected_flow || {}}
                      collapseToggle={this.collapse}
                      setSelectedFlow={this.setSelectedFlow}
                      client={this.props.client}/>
                    <FlowInspector
                      flow={this.state.currentFlow}
                      client={this.props.client}/>
                  </SplitPane>
                </ObserveKeys>
              </HotKeys>
            </>
        );
    }
};


export default withRouter(ClientFlowsView);
