import React from 'react';
import PropTypes from 'prop-types';

import NotebookCellRenderer from './notebook-cell-renderer.jsx';
import NotebookNavigator from './notebook-navigator.jsx';
import Spinner from '../utils/spinner.jsx';
import _ from 'lodash';
import T from '../i8n/i8n.jsx';

import api from '../core/api-service.jsx';
import  {CancelToken} from 'axios';

export default class NotebookRenderer extends React.Component {
    static propTypes = {
        env: PropTypes.object,
        notebook: PropTypes.object,
        updateVersion: PropTypes.func.isRequired,
    };

    state = {
        selected_cell_id: "",
        loading: false,

        // A locked notebook can not be edited. Each time a cell is in
        // flight, we increment the lock count until the notebook is in
        // a valid state, then the lock is decremented. This ensures
        // notebooks can only be edited in places where the server
        // acknowledges the current state.
        locked: 0,
        refs: {},
    }

    setSelectedCellId = (cell_id) => {
        this.setState({selected_cell_id: cell_id});
    }


    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    upCell = (cell_id) => {
        let notebook = Object.assign({}, this.props.notebook);
        let cell_metadata = [...notebook.cell_metadata];
        let changed = false;

        let new_cells = [];
        for (var i=0; i<cell_metadata.length; i++) {
            if (cell_metadata[i].cell_id === cell_id && new_cells.length > 0) {
                let last_cell = new_cells.pop();
                new_cells.push(cell_metadata[i]);
                new_cells.push(last_cell);
                changed = true;
            } else {
                new_cells.push(cell_metadata[i]);
            }
        }

        if (changed) {
            notebook.cell_metadata = new_cells;
            this.setState({loading: true});
            api.post('v1/UpdateNotebook', notebook,
                     this.source.token).then(response=>{
                         if (response.cancel) return;
                         this.props.updateVersion();
                         this.setState({loading: false, locked: 0});
                     }).catch(e=> {
                         this.setState({loading: false, locked: 0});
                         this.props.updateVersion();
                     });
        }
    };

    deleteCell = (cell_id) => {
        let changed = false;
        let notebook = Object.assign({}, this.props.notebook);
        let cell_metadata = [...notebook.cell_metadata];

        // Dont allow us to remove all cells.
        if (cell_metadata.length <= 1) {
            return;
        }

        var new_cells = [];
        for (var i=0; i<cell_metadata.length; i++) {
            if (cell_metadata[i].cell_id === cell_id) {
                changed = true;
            } else {
                new_cells.push(cell_metadata[i]);
            }
        }

        if (changed) {
            notebook.cell_metadata = new_cells;
            this.setState({loading: true});
            api.post('v1/UpdateNotebook', notebook,
                     this.source.token).then(response=>{
                         if (response.cancel) return;

                         this.props.updateVersion();
                         this.setState({loading: false, locked: 0});
                     }).catch(e=>{
                         this.setState({loading: false, locked: 0});
                         this.props.updateVersion();
                     });
        }
    };

    downCell = (cell_id) => {
        var changed = false;
        let notebook = Object.assign({}, this.props.notebook);
        var cell_metadata = [...notebook.cell_metadata];

        var new_cells = [];
        for (var i=0; i<cell_metadata.length; i++) {
            if (cell_metadata[i].cell_id === cell_id && cell_metadata.length > i) {
                var next_cell = cell_metadata[i+1];
                if (!_.isEmpty(next_cell)) {
                    new_cells.push(next_cell);
                    new_cells.push(cell_metadata[i]);
                    i += 1;
                    changed = true;
                }
            } else {
                new_cells.push(cell_metadata[i]);
            }
        }

        if (changed) {
            notebook.cell_metadata = new_cells;
            this.setState({loading: true});
            api.post('v1/UpdateNotebook', notebook,
                     this.source.token).then(response=>{
                         if (response.cancel) return;
                         this.props.updateVersion();
                         this.setState({loading: false, locked: 0});
                     }).catch(e=>{
                         this.setState({loading: false, locked: 0});
                         this.props.updateVersion();
                     });
        }
    };

    addCell = (cell_id, cell_type, content, env) => {
        let request = {};
        switch(cell_type.toLowerCase()) {
        case "vql":
        case "markdown":
        case "artifact":
            request = {
                notebook_id: this.props.notebook.notebook_id,
                type: cell_type,
                cell_id: cell_id,
                env: env,
                input: content,
            }; break;
        default:
            return;
        }

        this.setState({loading: true});
        api.post('v1/NewNotebookCell',
                 request,
                 this.source.token).then((response) => {
                     if (response.cancel) return;
                     this.props.updateVersion();
                     this.setState({selected_cell_id: response.data.latest_cell_id,
                                    loading: false});
                 });
    }

    scrollToCell = cell_id=>{
        let ref = this.getRef(cell_id);
        if(ref && ref.current && ref.current.scrollRef) {
            ref.current.scrollRef.current.scrollIntoView({
                behavior: "smooth",
            });
        }
    }

    getRef = cell_id=>{
        let res = this.state.refs[cell_id];
        if(!res) {
            res = React.createRef();
            this.state.refs[cell_id] = res;
        }
        return res;
    }

    render() {
        if (!this.props.notebook || _.isEmpty(this.props.notebook.cell_metadata)) {
            return <h5 className="no-content">
                     {T("Select a notebook from the list above.")}
                   </h5>;
        }

        return (
            <>
              <NotebookNavigator
                scrollToCell={this.scrollToCell}
                notebook={this.props.notebook}
              />
              <Spinner loading={this.state.loading || this.props.notebook.loading} />
              <div className="notebook-contents">
              { _.map(this.props.notebook.cell_metadata, (cell_md, idx) => {
                  return <NotebookCellRenderer
                           env={this.props.env}
                           ref={this.getRef(cell_md.cell_id)}
                           selected_cell_id={this.state.selected_cell_id}
                           setSelectedCellId={this.setSelectedCellId}
                           notebook_id={this.props.notebook.notebook_id}
                           notebook_metadata={this.props.notebook}
                           cell_metadata={cell_md} key={idx}
                           updateVersion={this.props.updateVersion}
                           upCell={this.upCell}
                           downCell={this.downCell}
                           deleteCell={this.deleteCell}
                           addCell={this.addCell}
                           notebookLocked={this.state.locked}
                           incNotebookLocked={x=>this.setState(
                               prevState=>{
                                   // Atomic version of setState
                                   // https://www.digitalocean.com/community/tutorials/react-getting-atomic-updates-with-setstate
                                   return {locked: x+prevState.locked};
                               })}
                      />;
              })}
              </div>
            </>
        );
    }
};
