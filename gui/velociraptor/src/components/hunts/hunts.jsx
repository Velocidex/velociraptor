import React from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import HuntList from './hunt-list.jsx';
import HuntInspector from './hunt-inspector.jsx';
import _ from 'lodash';
import api from '../core/api-service.jsx';

import {getItem, setItem, schema} from '../core/storage.jsx';
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
        selected_hunt_id: undefined,

        // The full detail of the current selected hunt.
        selected_hunt: {},

        // A list of hunt summary objects - contains just enough info
        // to render tables.
        hunts: [],

        filter: "",

        collapsed: false,
        topPaneSize: "30%",
    }

    componentDidMount = () => {
        this.get_hunts_source = CancelToken.source();
        this.fetchSelectedHunt();
    }

    componentWillUnmount() {
        this.get_hunts_source.cancel();
    }

    collapse = level=> {
        setItem(schema.HuntSplitKey, level);
        this.setState({topPaneSize: level});
    }

    setSelectedHuntId = (hunt_id) => {
        if (!hunt_id) {
            this.setState({
                selected_hunt: {},
                selected_hunt_id: undefined,
            });
            this.props.history.push("/hunts");
            return;
        }

        let tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        if(!tab) {
            tab = getItem(schema.CurrentHuntTabKey);
        }

        if (tab) {
            this.props.history.push("/hunts/" + hunt_id + "/" + tab);
        } else {
            this.props.history.push("/hunts/" + hunt_id);
        }
        setItem(schema.CurrentHuntIdKey, hunt_id);
        this.setState({selected_hunt_id: hunt_id});
        this.loadHunt(hunt_id);
    }

    loadHunt = (hunt_id) => {
        if(!hunt_id){
            hunt_id = this.state.selected_hunt_id;
        };

        if(!hunt_id || hunt_id === "new") {
            return;
        }

        this.get_hunts_source.cancel();
        this.get_hunts_source = CancelToken.source();

        api.get("v1/GetHunt", {
            hunt_id: hunt_id,
        }, this.get_hunts_source.token).then((response) => {
            if (response.cancel) return;

            if(_.isEmpty(response.data)) {
                this.setState({selected_hunt: {}});
            } else {
                this.setState({selected_hunt: response.data});
            }

        }).catch(response=>{
            // The current hunt is not found, navigate away from it.
            let status = response.response && response.response.status;
            if(status === 404) {
                setItem(schema.CurrentHuntIdKey, "");
                this.props.history.push("/hunts");
            };
        });
    }

    fetchSelectedHunt = () => {
        let selected_hunt_id = this.props.match &&
            this.props.match.params &&
            this.props.match.params.hunt_id;

        if (!selected_hunt_id) {
            selected_hunt_id = getItem(schema.CurrentHuntIdKey);
        }

        if (!selected_hunt_id) return;

        setItem(schema.CurrentHuntIdKey, selected_hunt_id);
        this.loadHunt(selected_hunt_id);
    }

    render() {
        let size = getItem(schema.HuntSplitKey) || this.state.topPaneSize;

        return (
            <>
              <SplitPane
                onChange={size=>{
                    setItem(schema.HuntSplitKey, size + "px");
                }}
                split="horizontal"
                defaultSize="30%"
                size={size}>
                <HuntList
                  collapseToggle={this.collapse}
                  updateHunts={this.fetchSelectedHunt}
                  selected_hunt={this.state.selected_hunt}
                  setSelectedHuntId={this.setSelectedHuntId} />
                <HuntInspector
                  fetch_hunts={this.fetchSelectedHunt}
                  hunt={this.state.selected_hunt} />
              </SplitPane>
            </>
        );
    }
};

export default withRouter(VeloHunts);
