import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';

import FlowsList from './flows-list.js';
import FlowInspector from "./flows-inspector.js";
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';
import axios from 'axios';

const POLL_TIME = 5000;

class ClientFlowsView extends React.Component {
    static propTypes = {
        client: PropTypes.object,
    };

    state = {
        flows: [],
        currentFlow: {},
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchFlows, POLL_TIME);
        this.fetchFlows();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let old_client_id = prevProps.client && prevProps.client.client_id;
        let new_client_id = this.props.client.client_id;

        if (old_client_id !== new_client_id) {
            this.fetchFlows();
        }
    }

    fetchFlows = () => {
        let client_id = this.props.client && this.props.client.client_id;
        if (!client_id) {
            return;
        }

        let selected_flow_id = this.props.match && this.props.match.params &&
            this.props.match.params.flow_id;

        api.get("api/v1/GetClientFlows/" + client_id, {
            count: 100,
            offset: 0,
        }).then(function(response) {
            let flows = response.data.items || [];
            let selected_flow = {};

            // If the router specifies a selected flow id, we select it.
            for(var i=0;i<flows.length;i++) {
                let flow=flows[i];
                if (flow.session_id === selected_flow_id) {
                    selected_flow = flow;
                    break;
                }
            };

            this.setState({flows: flows, currentFlow: selected_flow});
        }.bind(this));
    }

    setSelectedFlow = (flow) => {
        this.setState({currentFlow: flow});

        // Update the route.
        this.props.history.push(
            "/collected/" + this.props.client.client_id + "/" + flow.session_id);
    }

    render() {
        return (
            <>
              <SplitPane split="horizontal" defaultSize="30%">
                <FlowsList
                  selected_flow={this.state.currentFlow}
                  flows={this.state.flows}
                  fetchFlows={this.fetchFlows}
                  setSelectedFlow={this.setSelectedFlow}
                  client={this.props.client}/>
                <FlowInspector
                  flow={this.state.currentFlow}
                  client={this.props.client}/>
              </SplitPane>
            </>
        );
    }
};


export default withRouter(ClientFlowsView);
