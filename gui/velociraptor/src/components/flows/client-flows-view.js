import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import SplitPane from 'react-split-pane';

import FlowsList from './flows-list.js';
import FlowInspector from "./flows-inspector.js";
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';
import axios from 'axios';
import Spinner from '../utils/spinner.js';

const POLL_TIME = 5000;

class ClientFlowsView extends React.Component {
    static propTypes = {
        client: PropTypes.object,
    };

    state = {
        flows: [],
        currentFlow: {},

        // Only show the spinner when the component is first mounted.
        loading: true,
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

        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get("v1/GetClientFlows/" + client_id, {
            count: 1000,
            offset: 0,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let flows = response.data.items || [];
            let selected_flow_id = this.state.currentFlow.session_id;

            // If the router specifies a selected flow id, we select it.
            if (!this.state.init_router) {
                selected_flow_id = this.props.match && this.props.match.params &&
                    this.props.match.params.flow_id;

                // If we can not find the selected_flow we just select the first one
                if (!selected_flow_id && !_.isEmpty(flows)){
                    selected_flow_id = flows[0].session_id;
                }

                this.setState({init_router: true});
            }

            // Update the current selected flow with the new data.
            if (selected_flow_id) {
                _.each(flows, flow=>{
                    if(flow.session_id === selected_flow_id) {
                        this.setState({currentFlow: flow});
                    };
                });
            }

            this.setState({flows: flows, loading: false});
        });
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

    render() {
        return (
            <>
              <Spinner loading={this.state.loading} />
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
