import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import HuntList from './hunt-list.jsx';
import HuntInspector from './hunt-inspector.jsx';
import _ from 'lodash';
import api from '../core/api-service.jsx';

import  {CancelToken} from 'axios';

import { withRouter }  from "react-router-dom";

class VeloHunts extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        // The currently selected hunt summary.
        selected_hunt_id: {},

        // The full detail of the current selected hunt.
        full_selected_hunt: {},

        // A list of hunt summary objects - contains just enough info
        // to render tables.
        hunts: [],

        filter: "",

        collapsed: false,
        topPaneSize: "30%",
    }

    componentDidMount = () => {
        this.get_hunts_source = CancelToken.source();
    }

    componentWillUnmount() {
        this.get_hunts_source.cancel();
    }

    collapse = level=> {
        this.setState({topPaneSize: level});
    }

    setSelectedHuntId = (hunt_id) => {
        if (!hunt_id) {
            this.setState({
                full_selected_hunt: {},
                selected_hunt_id: undefined,
            });
            this.props.history.push("/hunts");
            return;
        }

        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        if (tab) {
            this.props.history.push("/hunts/" + hunt_id + "/" + tab);
        } else {
            this.props.history.push("/hunts/" + hunt_id);
        }
        this.setState({selected_hunt_id: hunt_id});
        this.loadFullHunt(hunt_id);
    }

    loadFullHunt = (hunt_id) => {
        if(!hunt_id){
            return;
        };

        this.get_hunts_source.cancel();
        this.get_hunts_source = CancelToken.source();

        api.get("v1/GetHunt", {
            hunt_id: hunt_id,
        }, this.get_hunts_source.token).then((response) => {
            if (response.cancel) return;

            if(_.isEmpty(response.data)) {
                this.setState({full_selected_hunt: {}});
            } else {
                this.setState({full_selected_hunt: response.data});
            }
        });
    }

    fetchSelectedHunt = () => {
        let selected_hunt_id = this.props.match && this.props.match.params &&
            this.props.match.params.hunt_id;

        if (!selected_hunt_id) return;
        this.loadFullHunt(selected_hunt_id);
    }

    render() {
        return (
            <>
              <SplitPane
                size={this.state.topPaneSize}
                split="horizontal"
                defaultSize="30%">
                <HuntList
                  collapseToggle={this.collapse}
                  updateHunts={this.fetchSelectedHunt}
                  selected_hunt={this.state.full_selected_hunt}
                  setSelectedHuntId={this.setSelectedHuntId} />
                <HuntInspector
                  fetch_hunts={this.fetchSelectedHunt}
                  hunt={this.state.full_selected_hunt} />
              </SplitPane>
            </>
        );
    }
};

export default withRouter(VeloHunts);
