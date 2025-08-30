import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import FlowsList from './flows-list.jsx';
import FlowInspector from "./flows-inspector.jsx";
import { withRouter }  from "react-router-dom";
import { HotKeys, ObserveKeys } from "react-hotkeys";

import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';


class ClientFlowsView extends React.Component {
    static propTypes = {
        client: PropTypes.object,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        currentFlow: {},
        topPaneSize: undefined,
        selectedMultiFlows: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();

        let flow_id = this.props.match && this.props.match.params &&
            this.props.match.params.flow_id;

        if(flow_id && flow_id !== "new") {
            let client_id = this.props.match && this.props.match.params &&
                this.props.match.params.client_id;

            api.get("v1/GetFlowDetails", {
                flow_id: flow_id,
                client_id: client_id || "server",
            }, this.source.token).then((response) => {
                this.setState({currentFlow: response.data.context});
            });
        }
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setSelectedFlow = (flow) => {
        this.setState({currentFlow: flow});

        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        // Update the route.
        if (tab) {
            this.props.history.push(
                "/collected/" + this.props.client.client_id + "/" + flow.session_id + "/" + tab);
        } else {
            this.props.history.push(
                "/collected/" + this.props.client.client_id + "/" + flow.session_id);
        }
    }

    gotoTab = (tab) => {
        let client_id = this.props.client.client_id;
        let selected_flow = this.state.currentFlow && this.state.currentFlow.session_id;
        this.props.history.push(
            "/collected/" + client_id + "/" + selected_flow + "/" + tab);
    }

    setMultiSelectedFlow = flows=>{
        this.setState({selectedMultiFlows: flows});
    }

    collapse = level=> {
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


        return (
            <>
              <HotKeys keyMap={KeyMap} handlers={keyHandlers}>
                <ObserveKeys>
                  <SplitPane split="horizontal"
                             size={this.state.topPaneSize}
                             onResizerDoubleClick={x=>this.collapse("50%")}
                             defaultSize="80%">
                    <FlowsList
                      selected_flow={this.state.currentFlow}
                      collapseToggle={this.collapse}
                      setSelectedFlow={this.setSelectedFlow}
                      setMultiSelectedFlow={this.setMultiSelectedFlow}
                      client={this.props.client}/>
                    <FlowInspector
                      flow={this.state.currentFlow}
                      client={{client_id: this.props.client}}/>
                  </SplitPane>
                </ObserveKeys>
              </HotKeys>
            </>
        );
    }
};


export default withRouter(ClientFlowsView);
