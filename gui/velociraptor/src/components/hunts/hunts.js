import React from 'react';
import PropTypes from 'prop-types';

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
        current_hunt: {},
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
        this.setState({current_hunt: hunt});
        this.props.history.push("/hunts/" + hunt.hunt_id);
    }

    fetchHunts = () => {
        let selected_hunt_id = this.props.match && this.props.match.params &&
            this.props.match.params.hunt_id;

        api.get("api/v1/ListHunts", {
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

            this.setState({hunts: hunts, current_hunt: selected_hunt});
        });

    }

    render() {
        return (
            <SplitPane split="horizontal" defaultSize="30%">
              <HuntsList
                selected_hunt={this.state.current_hunt}
                hunts={this.state.hunts}
                setSelectedHunt={this.setSelectedHunt} />
              <HuntInspector
                hunt={this.state.current_hunt} />
            </SplitPane>

        );
    }
};

export default withRouter(VeloHunts);
