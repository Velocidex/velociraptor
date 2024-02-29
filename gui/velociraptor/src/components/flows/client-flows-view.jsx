import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import FlowsList from './flows-list.jsx';
import FlowInspector from "./flows-inspector.jsx";
import { withRouter }  from "react-router-dom";

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

    collapse = level=> {
        this.setState({topPaneSize: level});
    }

    render() {
        return (
            <>
              <SplitPane split="horizontal"
                         size={this.state.topPaneSize}
                         onResizerDoubleClick={x=>this.collapse("50%")}
                         defaultSize="80%">
                <FlowsList
                  selected_flow={this.state.currentFlow}
                  collapseToggle={this.collapse}
                  setSelectedFlow={this.setSelectedFlow}
                  client={this.props.client}/>
                <FlowInspector
                  flow={this.state.currentFlow}
                  client={{client_id: this.props.client}}/>
              </SplitPane>
            </>
        );
    }
};


export default withRouter(ClientFlowsView);
