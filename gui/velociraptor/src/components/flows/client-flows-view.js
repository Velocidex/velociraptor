import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';

import FlowsList from './flows-list.js';
import FlowInspector from "./flows-inspector.js";

import api from '../core/api-service.js';

export default class ClientFlowsView extends React.Component {
    static propTypes = {
        client: PropTypes.object,
    };

    state = {
        flows: [],
        currentFlow: {},
    }

    componentDidMount = () => {
        this.fetchFlows();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let old_client_id = prevProps.client && prevProps.client.client_id;
        let new_client_id = this.props.client.client_id;

        if (old_client_id != new_client_id) {
            this.fetchFlows();
        }
    }

    fetchFlows = () => {
        let client_id = this.props.client && this.props.client.client_id;
        if (!client_id) {
            return;
        }

        api.get("api/v1/GetClientFlows/" + client_id, {
            count: 100,
            offset: 0,
        }).then(function(response) {
            this.setState({flows: response.data.items});
        }.bind(this));
    }

    setSelectedFlow = (flow) => {
        this.setState({currentFlow: flow});
    }

    render() {
        return (
            <>
              <SplitPane split="horizontal" defaultSize="30%">
                <FlowsList
                  flows={this.state.flows}
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
