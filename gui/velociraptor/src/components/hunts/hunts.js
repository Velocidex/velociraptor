import React from 'react';
import SplitPane from 'react-split-pane';
import HuntList from './hunt-list.js';
import HuntInspector from './hunt-inspector.js';
import _ from 'lodash';
import api from '../core/api-service.js';

import axios from 'axios';
import Spinner from '../utils/spinner.js';

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

        loading: true,
    }

    componentDidMount = () => {
        this.get_hunts_source = axios.CancelToken.source();
        this.list_hunts_source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchHunts, POLL_TIME);
        this.fetchHunts();
    }

    componentWillUnmount() {
        this.list_hunts_source.cancel();
        this.get_hunts_source.cancel();
        clearInterval(this.interval);
    }

    setSelectedHunt = (hunt) => {
        if (!hunt || !hunt.hunt_id) {
            return;
        }
        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        if (tab) {
            this.props.history.push("/hunts/" + hunt.hunt_id + "/" + tab);
        } else {
            this.props.history.push("/hunts/" + hunt.hunt_id);
        }
        this.loadFullHunt(hunt);
    }

    loadFullHunt = (hunt) => {
        this.setState({selected_hunt: hunt});
        this.get_hunts_source.cancel();
        this.get_hunts_source = axios.CancelToken.source();
        this.setState({full_selected_hunt: {loading: true}});

        api.get("v1/GetHunt", {
            hunt_id: hunt.hunt_id,
        }, this.get_hunts_source.token).then((response) => {
            if (response.cancel) return;

            if(_.isEmpty(response.data)) {
                this.setState({full_selected_hunt: {loading: true}});
            } else {
                this.setState({full_selected_hunt: response.data});
            }
        });
    }

    fetchHunts = () => {
        let selected_hunt_id = this.props.match && this.props.match.params &&
            this.props.match.params.hunt_id;

        this.list_hunts_source.cancel();
        this.list_hunts_source = axios.CancelToken.source();

        // Some users have a lot of hunts and listing that many might
        // be prohibitively expensive.
        api.get("v1/ListHunts", {
            count: 2000,
            offset: 0,
        }, this.list_hunts_source.token).then((response) => {
            if (response.cancel) return;

            let hunts = response.data.items || [];
            let selected_hunt_id = this.state.selected_hunt && this.state.selected_hunt.hunt_id;

            // If the router specifies a selected flow id, we select
            // it but only once.
            if (!this.state.init_router && !selected_hunt_id) {
                selected_hunt_id = this.props.match && this.props.match.params &&
                    this.props.match.params.hunt_id;

                this.setState({init_router: true});
            }

            // Update the selected hunt in the state.
            if (selected_hunt_id) {
                _.each(hunts, hunt=>{
                    if (_.isUndefined(hunt.hunt_description)) {
                        hunt.hunt_description = "";
                    }

                    if (hunt.hunt_id === selected_hunt_id) {
                        this.setState({selected_hunt: hunt});
                    };
                });
            };

            this.setState({hunts: hunts, loading: false});
        });

        // Get the full hunt information from the server based on the
        // hunt metadata - this is done every poll interval to ensure
        // hunt stats are updated.
        if (selected_hunt_id) {
            this.get_hunts_source.cancel();
            this.get_hunts_source = axios.CancelToken.source();

            api.get("v1/GetHunt", {
                hunt_id: selected_hunt_id,
            }, this.get_hunts_source.token).then((response) => {
                if (response.cancel) return;

                if(_.isEmpty(response.data)) {
                    this.setState({full_selected_hunt: {loading: true}});
                } else {
                    this.setState({full_selected_hunt: response.data});
                }

            });
        }
    }

    render() {
        return (
            <><Spinner loading={this.state.loading}/>
              <SplitPane split="horizontal" defaultSize="30%">
                <HuntList
                  updateHunts={this.fetchHunts}
                  selected_hunt={this.state.selected_hunt}
                  hunts={this.state.hunts}
                  setSelectedHunt={this.setSelectedHunt} />
                <HuntInspector
                  hunt={this.state.full_selected_hunt} />
              </SplitPane>
            </>
        );
    }
};

export default withRouter(VeloHunts);
