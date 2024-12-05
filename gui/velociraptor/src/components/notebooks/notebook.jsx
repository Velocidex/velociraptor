import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import NotebooksList from './notebooks-list.jsx';
import NotebookRenderer from './notebook-renderer.jsx';

import SplitPane from 'react-split-pane';

import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import Spinner from '../utils/spinner.jsx';
import { withRouter }  from "react-router-dom";

// Poll for new notebooks list.
const POLL_TIME = 5000;


class Notebooks extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        selected_notebook: {},
        version: 0,
        loading: false,
        refreshed_from_router: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.updateVersion, POLL_TIME);
        this.updateVersion();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    updateVersion = ()=>{
        this.setState({version: this.state.version+1});
        let notebook_id = this.state.selected_notebook &&
            this.state.selected_notebook.notebook_id;

        if(!_.isEmpty(notebook_id)) {
            this.setSelectedNotebook(notebook_id);
        } else {
            let match_notebook_id = this.props.match &&
                this.props.match.params &&
                this.props.match.params.notebook_id;
            if (match_notebook_id) {
                this.setSelectedNotebook(match_notebook_id);
            }
        }
    }

    setSelectedNotebook = (notebook_id) => {
        // Fetch the data again with more details this time
        this.source.cancel();
        this.source = CancelToken.source();

        // Only show the loading sign when we load a new notebook not
        // for periodic refresh.
        let old_notebook_id = this.state.selected_notebook &&
            this.state.selected_notebook.notebook_id;
        if (notebook_id !== old_notebook_id) {
            this.setState({loading: true});
        }

        api.get("v1/GetNotebooks", {
            notebook_id: notebook_id,
        }, this.source.token).then(response=>{
            this.setState({loading: false});

            if (response.cancel) return;

            let notebooks = response.data.items || [];

            if (notebooks.length > 0) {
                let selected_notebook = notebooks[0];
                let current_selected_notebook = this.state.selected_notebook || {};

                // Only modify the notebook if it has changed
                if (selected_notebook.modified_time != current_selected_notebook.modified_time) {
                    this.setState({
                        selected_notebook: selected_notebook,
                        loading: false});
                    this.props.history.push("/notebooks/" + notebooks[0].notebook_id);
                }
            }
        });
    };

    render() {
        return (
            <>
              <Spinner loading={this.state.loading} />
              <SplitPane split="horizontal" defaultSize="30%">
                <NotebooksList
                  updateVersion={this.updateVersion}
                  version={this.state.version}
                  selected_notebook={this.state.selected_notebook}
                  setSelectedNotebook={this.setSelectedNotebook}
                />
                <NotebookRenderer
                  updateVersion={this.updateVersion}
                  notebook={this.state.selected_notebook}
                />
              </SplitPane>
            </>
        );
    }
};

export default withRouter(Notebooks);
