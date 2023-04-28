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
            let selected_notebook = {};

            // Check the router for a notebook id
            let notebook_id = this.props.match && this.props.match.params &&
                this.props.match.params.notebook_id;
            if (notebook_id) {
                for(var i = 0; i < notebooks.length; i++){
                    if (notebooks[i].notebook_id === notebook_id) {
                        selected_notebook = notebooks[i];
                        break;
                    }
                }
            }

            this.setState({notebooks: notebooks,
                           loading: false,
                           selected_notebook: selected_notebook});
        });
    }

    setSelectedNotebook = (notebook) => {
        notebook.loading = true;
        this.setState({selected_notebook: notebook});
        this.props.history.push("/notebooks/" + notebook.notebook_id);
    }

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
