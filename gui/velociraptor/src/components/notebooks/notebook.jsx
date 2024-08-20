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
const PAGE_SIZE = 100;


class Notebooks extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        notebooks: [],
        selected_notebook: {},

        // Only show the spinner the first time the component is
        // mounted.
        loading: true,

        refreshed_from_router: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchNotebooks, POLL_TIME);
        this.fetchNotebooks();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    fetchNotebooks = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetNotebooks", {
            count: PAGE_SIZE,
            offset: 0,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let notebooks = response.data.items || [];

            this.setState({notebooks: notebooks, loading: false});

            let selected_notebook_id = this.state.selected_notebook &&
                this.state.selected_notebook.notebook_id;

            // Check the router for a notebook id
            if (!this.state.refreshed_from_router) {
                let router_notebook_id = this.props.match && this.props.match.params &&
                    this.props.match.params.notebook_id;
                if (router_notebook_id) {
                    selected_notebook_id = router_notebook_id;
                }
                this.setState({refreshed_from_router: true});
            }

            // Now search the notebooks records to find the selected
            // notebook record. We need to get its last modified time to figure
            // out if it needs reloading.
            let selected_brief_record = {};
            for(let i = 0; i < notebooks.length; i++) {
                if (notebooks[i].notebook_id === selected_notebook_id) {
                    selected_brief_record = notebooks[i];
                    break;
                }
            }

            // Only reload the full version if the notebook is modified.
            if (this.state.selected_notebook.modified_time !==
                selected_brief_record.modified_time) {
                this.setSelectedNotebook({notebook_id: selected_notebook_id});
            };
        });
    }

    setSelectedNotebook = (notebook) => {
        // Fetch the data again with more details this time
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetNotebooks", {
            notebook_id: notebook.notebook_id,
            include_uploads: true,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let notebooks = response.data.items || [];

            if (notebooks.length > 0) {
                let selected_notebook = notebooks[0];
                let current_selected_notebook = this.state.selected_notebook || {};
                // Only modify the notebook if it has changed
                if (selected_notebook.modified_time != current_selected_notebook.modified_time) {
                    this.setState({selected_notebook: notebooks[0], loading: false});
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
                  fetchNotebooks={this.fetchNotebooks}
                  selected_notebook={this.state.selected_notebook}
                  setSelectedNotebook={this.setSelectedNotebook}
                  notebooks={this.state.notebooks}
                />
                <NotebookRenderer
                  fetchNotebooks={this.fetchNotebooks}
                  notebook={this.state.selected_notebook}
                />
              </SplitPane>
            </>
        );
    }
};

export default withRouter(Notebooks);
