import React from 'react';

import SplitPane from 'react-split-pane';
import _ from 'lodash';
import FlowsList from './flows-list.js';
import FlowInspector from "./flows-inspector.js";
import { withRouter } from "react-router-dom";

import api from '../core/api-service.js';
import axios from 'axios';
import Spinner from '../utils/spinner.js';

const POLL_TIME = 5000;

class ServerFlowsView extends React.Component {
  static propTypes = {};

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

  fetchFlows = () => {
    let selected_flow_id = this.props.match && this.props.match.params &&
      this.props.match.params.flow_id;

    // Cancel any in flight calls.
    this.source.cancel();
    this.source = axios.CancelToken.source();

    api.get("v1/GetClientFlows/server", {
        count: 100,
        offset: 0,
      }, this.source.token)
      .then(response => {
        if (response.cancel) return;

        let flows = response.data.items || [];
        let selected_flow = {};

        // If the router specifies a selected flow id, we select it.
        if (!this.state.init_router) {
          for (var i = 0; i < flows.length; i++) {
            let flow = flows[i];
            if (flow.session_id === selected_flow_id) {
              selected_flow = flow;
              break;
            }
          };

          // If we can not find the selected_flow we just select the first one
          if (_.isEmpty(selected_flow) && !_.isEmpty(flows)) {
            selected_flow = flows[0];
          }

          this.setState({
            init_router: true,
            currentFlow: selected_flow
          });
        }

        this.setState({ flows: flows, loading: false });
      });
  }

  setSelectedFlow = (flow) => {
    this.setState({ currentFlow: flow });

    let tab = this.props.match && this.props.match.params && this.props
      .match.params.tab;
    // Update the route.
    if (tab) {
      this.props.history.push(
        "/collected/server/" + flow.session_id + "/" + tab);
    } else {
      this.props.history.push(
        "/collected/server/" + flow.session_id);
    }
  }

  render() {
    return (
      <>
        <Spinner loading={this.state.loading} /> {
        !this.props.fullscreen ?
          <SplitPane split="horizontal" defaultSize="30%">
                    <FlowsList
                    selected_flow={this.state.currentFlow}
                    flows={this.state.flows}
                    fetchFlows={this.fetchFlows}
                    setSelectedFlow={this.setSelectedFlow}
                    client={{client_id: "server"}}/>
                    <FlowInspector
                    flow={this.state.currentFlow}
                      client={{client_id: "server"}}
                      fullscreen={this.props.fullscreen}
                      toggleFullscreen={this.props.toggleFullscreen}/>
                </SplitPane> :
          <FlowInspector
                flow={this.state.currentFlow}
                  client={{client_id: "server"}}
                  fullscreen={this.props.fullscreen}
                  toggleFullscreen={this.props.toggleFullscreen}/>
      } <
      />
    );
  }
};

export default withRouter(ServerFlowsView);
