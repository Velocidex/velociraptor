import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import FlowsList from './flows-list.jsx';
import FlowInspector from "./flows-inspector.jsx";
import { withRouter }  from "react-router-dom";


class ServerFlowsView extends React.Component {
    static propTypes = {
        collapseToggle: PropTypes.func,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        flows: [],
        currentFlow: {},
        topPaneSize: undefined,
    }

    setSelectedFlow = (flow) => {
        this.setState({currentFlow: flow});

        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        // Update the route.
        if (tab) {
            this.props.history.push(
                "/collected/server/" + flow.session_id + "/" + tab);
        } else {
            this.props.history.push(
                "/collected/server/" + flow.session_id);
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
                         defaultSize="30%">
                <FlowsList
                  selected_flow={this.state.currentFlow}
                  flows={this.state.flows}
                  fetchFlows={this.fetchFlows}
                  collapseToggle={this.collapse}
                  setSelectedFlow={this.setSelectedFlow}
                  client={{client_id: "server"}}/>
                <FlowInspector
                  flow={this.state.currentFlow}
                  client={{client_id: "server"}}/>
              </SplitPane>
            </>
        );
    }
};

export default withRouter(ServerFlowsView);
