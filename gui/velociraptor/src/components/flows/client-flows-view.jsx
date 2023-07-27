import React from 'react';
import PropTypes from 'prop-types';
import SplitPane from 'react-split-pane';

import FlowsList from './flows-list.jsx';
import FlowInspector from "./flows-inspector.jsx";
import { withRouter }  from "react-router-dom";


class ClientFlowsView extends React.Component {
    static propTypes = {
        client: PropTypes.object,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        currentFlow: {},

        // Only show the spinner when the component is first mounted.
        loading: true,
        topPaneSize: "30%",
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
                  client={this.props.client}/>
              </SplitPane>
            </>
        );
    }
};


export default withRouter(ClientFlowsView);
