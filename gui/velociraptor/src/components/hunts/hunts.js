import React from 'react';
import SplitPane from 'react-split-pane';
import HuntsList from './hunt-list.js';
import HuntInspector from './hunt-inspector.js';

import api from '../core/api-service.js';

import axios from 'axios';

import { withRouter }  from "react-router-dom";

const POLL_TIME = 5000;

class VeloHunts extends React.Component {
    static propTypes = {

    };

    state = {
        // The currently selected hunt summary.
        selected_hunt: {},

        // The full detail of the current selected hunt.
        full_selected_hunt: {},

        // A list of hunt summary objects - contains just enough info
        // to render tables.
        hunts: [],
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchHunts, POLL_TIME);
        this.fetchHunts();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    setSelectedHunt = (hunt) => {
        if (!hunt || !hunt.hunt_id) {
            return;
        }

        this.setState({selected_hunt: hunt});
        this.props.history.push("/hunts/" + hunt.hunt_id);

        api.get("v1/GetHunt", {
            hunt_id: hunt.hunt_id,
        }).then((response) => {
            this.setState({full_selected_hunt: response.data});
        });
    }

    fetchHunts = () => {
        let selected_hunt_id = this.props.match && this.props.match.params &&
            this.props.match.params.hunt_id;

        api.get("v1/ListHunts", {
            count: 100,
            offset: 0,
        }).then((response) => {
            let hunts = response.data.items;
            let selected_hunt = {};

            // If the router specifies a selected flow id, we select it.
            for(var i=0;i<hunts.length;i++) {
                let hunt=hunts[i];
                if (hunt.hunt_id === selected_hunt_id) {
                    selected_hunt = hunt;
                    break;
                }
            };

            this.setState({hunts: hunts, selected_hunt: selected_hunt});
        });

        // Get the full hunt information from the server based on the hunt
        // metadata
        if (selected_hunt_id) {
            api.get("v1/GetHunt", {
                hunt_id: selected_hunt_id,
            }).then((response) => {
                this.setState({full_selected_hunt: response.data});
            });
        }
    }

    render() {
        return (
            <SplitPane split="horizontal" defaultSize="30%">
              <HuntsList
                updateHunts={this.fetchHunts}
                selected_hunt={this.state.selected_hunt}
                hunts={this.state.hunts}
                setSelectedHunt={this.setSelectedHunt} />
              <HuntInspector
                hunt={this.state.full_selected_hunt} />
            </SplitPane>

        );
    }
};

export default withRouter(VeloHunts);
